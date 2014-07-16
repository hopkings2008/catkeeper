package main

import (
	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/render"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	_ "net/url"
	"net/http"
	"log"
	"code.google.com/p/go.net/websocket"
	"encoding/base64"
	"net"
	"strconv"
	"fmt"

)

//TODO move all sql to model.go
func main() {
    /* init database */
    db, err := sql.Open("sqlite3", "/tmp/post_db.bin")
    if err != nil {
	    checkErr(err, "open database failed")
	    return
    }
    defer db.Close()

    scanning := false

    m := martini.Classic()
    m.Use(render.Renderer())


    m.Get("/", func(r render.Render){
	    r.HTML(200, "index" , nil)
    })

    m.Get("/create", func(r render.Render) {
	    var Name string
	    var IpAddress string

	    type HostInfo struct {
		    Name string
		    IpAddress string
	    }

	    var hosts []HostInfo

	    rows, err := db.Query("select Name,IpAddress from physicalmachine")
	    if err != nil {
		    reportError(r, err, "failed to find physicalmachine")
	    }
	    for rows.Next() {
		    rows.Scan(&Name, &IpAddress)
		    hosts = append(hosts, HostInfo{Name, IpAddress})
	    }
	    log.Println(hosts)
	    r.HTML(200, "create", hosts)
    })

    m.Post("/create", func(r render.Render, req *http.Request) {
	    r.HTML(200, "create", nil)
    })


    m.Get("/add", func(r render.Render) {
	    r.HTML(200, "add", nil)

    })
    //missing validation 
    m.Post("/add", func(r render.Render, req *http.Request) {
	    var existingname string
	    name := req.PostFormValue("Name")
	    description := req.PostFormValue("Description")
	    IPAddress := req.PostFormValue("IPAddress")
	    log.Println(name+description+IPAddress)
	    //TODO: add more validations
	    err := db.QueryRow("select name from physicalmachine where IpAddress = ?", IPAddress).Scan(&existingname)
	    switch {
		    case err == sql.ErrNoRows:
			    // good to insert the PhysicalMachine
			    db.Exec("insert into physicalmachine (Name, IpAddress, Description) values(?,?,?)", name, IPAddress,description)
			    r.Redirect("/")
		    case err != nil:
			    //other error happens
			    r.HTML(501,"error", err)
		    default:
			    //no error happens
			    r.HTML(501,"error",fmt.Errorf("already has the physical machine %s", existingname))

	   }

    })

    m.Get("/vm/(?P<id>[0-9]+)", func(r render.Render, params martini.Params){
	    id,_ := strconv.Atoi(params["id"])
	    vm := getVirtualMachine(db,id)
	    r.HTML(200, "vm", vm)
    })



    m.Post("/rescan", func(req *http.Request) {
	    if !scanning {
		    go func() {
			    scanning = true
			    log.Println("start to rescan")
			    RescanIPAddress(db)
			    scanning = false

		    }()
	    }
    })

    m.Post("/vm/(?P<id>[0-9]+)", func(r render.Render, params martini.Params, req *http.Request) {
	    req.ParseForm()
	    submitType := req.PostFormValue("submit")
	    id,_ := strconv.Atoi(params["id"])
	    vm := getVirtualMachine(db,id)
	    var err error
	    switch submitType {
	    case "Start":
		    err = vm.Start()
		    vm.Free()
	    case "Stop":
		    err = vm.Stop()
		    //TODO: wait a while
		    vm.Free()
	    case "ForceStop":
		    err = vm.ForceStop()
		    vm.Free()
	    case "Update":
		    owner := req.PostForm["Owner"][0]
		    description := req.PostForm["Description"][0]
		    err = vm.UpdateDatabase(db, owner, description)
	    }

	    if err != nil {
		   //error page 
		   r.HTML(501, "error", err)
		   log.Println(err)
	    } else {
		    //redirect page
		    r.Redirect("/")
	    }
    })


    //API part
    m.Get("/api/list", func(r render.Render){
	    pm := getListofPhysicalMachineAndVirtualMachine (db)
	    r.JSON(200, pm)
    })

    wsConfig, _ := websocket.NewConfig("ws://127.0.0.1:3000", "http://127.0.0.1:3000")
    ws := websocket.Server{Handler:proxyHandler,
			    Config: *wsConfig, Handshake: func(ws *websocket.Config, req *http.Request) error {
			    ws.Protocol = []string{"base64"}
			    return nil
    }}


    m.Get("/websockify", ws.ServeHTTP)

    m.Run()
}

func proxyHandler(ws *websocket.Conn) {
	defer ws.Close()
	r := ws.Request()
	values := r.URL.Query()
	ip, hasIp := values["ip"]

	if hasIp == false {
		//log.Println("faile to parse vnc address")
		return
	}

	vc, err := net.Dial("tcp", ip[0])
	if err != nil {
		return
	}
	defer vc.Close()
	log.Println("new connection")
	done := make(chan bool)

	go func() {
		sbuf := make([]byte, 32*1024)
		dbuf := make([]byte, 32*1024)
		for {
			n, e := ws.Read(sbuf)
			if e != nil {
				done <- true
				return
			}
			n, e = base64.StdEncoding.Decode(dbuf, sbuf[0:n])
			if e != nil {
				done <- true
				return
			}
			n, e = vc.Write(dbuf[0:n])
			if e != nil {
				done <- true
				return
			}
		}
	}()
	go func() {
		sbuf := make([]byte, 32*1024)
		dbuf := make([]byte, 64*1024)
		for {
			n, e := vc.Read(sbuf)
			if e != nil {
				done <- true
				return
			}
			base64.StdEncoding.Encode(dbuf, sbuf[0:n])
			n = ((n + 2) / 3) * 4
			ws.Write(dbuf[0:n])
			if e != nil {
				done <- true
				return
			}
		}
	}()
	select {
	case <-done:
		break
	}
	log.Println("finish connection")
}

func reportError(r render.Render,err error, userError string) {
	r.HTML(501,"error", err.Error() + userError)
}

