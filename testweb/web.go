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
)

func main() {
    /* init database */
    db, err := sql.Open("sqlite3", "/tmp/post_db.bin")
    if err != nil {
	    checkErr(err, "open database failed")
	    return
    }
    defer db.Close()


    m := martini.Classic()
    m.Use(render.Renderer())


    m.Get("/", func(r render.Render){
	    pm := getListofPhysicalMachine(db)
	    r.HTML(200, "list" , pm)
    })


    wsConfig, _ := websocket.NewConfig("ws://127.0.0.1:3000", "http://127.0.0.1:3000")
    ws := websocket.Server{Handler:proxyHandler,
			    Config: *wsConfig,
                           Handshake: func(ws *websocket.Config, req *http.Request) error {
			    ws.Protocol = []string{"base64"}
			    return nil
    }}

    m.Get("/websockify", ws.ServeHTTP)

    m.Run()
}

func proxyHandler(ws *websocket.Conn) {
	r := ws.Request()
	values := r.URL.Query()
	ip, hasIp:= values["ip"]
	if !hasIp {
		log.Println("faile to parse vnc address")
		return
	}

	vc, err := net.Dial("tcp", ip[0])
	defer vc.Close()
	if err != nil {
		return
	}
	log.Println("new connection")
	go func() {
		sbuf := make([]byte, 32*1024)
		dbuf := make([]byte, 32*1024)
		for {
			n, e := ws.Read(sbuf)
			if e != nil {
				return
			}
			n, e = base64.StdEncoding.Decode(dbuf, sbuf[0:n])
			if e != nil {
				return
			}
			n, e = vc.Write(dbuf[0:n])
			if e != nil {
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
				return
			}
			base64.StdEncoding.Encode(dbuf, sbuf[0:n])
			n = ((n + 2) / 3) * 4
			ws.Write(dbuf[0:n])
			if e != nil {
				return
			}
		}
	}()
	select {}
}
