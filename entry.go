package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/samuelventura/go-tools"
	"github.com/samuelventura/go-tree"
)

func entry(node tree.Node) {
	dao := node.GetValue("dao").(Dao)
	crt := node.GetValue("server.crt").(string)
	key := node.GetValue("server.key").(string)
	//hostname := node.GetValue("hostname").(string)
	httpep := node.GetValue("http").(string)
	httpsep := node.GetValue("https").(string)
	dockep := node.GetValue("dock").(string)
	mainurls := node.GetValue("main").(string)
	httpsep_parts := strings.SplitN(httpsep, ":", 2)
	if len(httpsep_parts) != 2 {
		log.Panicf("httpsep must have 2 parts")
	}
	mainurl, err := url.Parse(mainurls)
	tools.PanicIfError(err)
	mainrp := httputil.NewSingleHostReverseProxy(mainurl)
	listen443, err := net.Listen("tcp", httpsep)
	tools.PanicIfError(err)
	node.AddCloser("listen443", listen443.Close)
	server443 := &http.Server{
		Addr:    httpsep,
		Handler: &server443Dso{mainrp, mainurl, dockep, dao},
	}
	node.AddProcess("server443", func() {
		err = server443.ServeTLS(listen443, crt, key)
		if err != nil {
			log.Println(httpsep, err)
		}
	})
	count80 := newCount()
	listen80, err := net.Listen("tcp", httpep)
	tools.PanicIfError(err)
	node.AddCloser("listen80", listen80.Close)
	handleConn80 := func(node tree.Node, conn net.Conn) {
		err := conn.SetDeadline(time.Now().Add(4 * time.Second))
		tools.PanicIfError(err)
		head, err := readLine(conn, 256)
		if err != nil {
			return
		}
		head_parts := strings.SplitN(head, " ", 3)
		if len(head_parts) != 3 {
			return
		}
		host := ""
		path := head_parts[1]
		for {
			header, err := readLine(conn, 1024)
			if err != nil {
				return
			}
			if strings.HasPrefix(header, "Host:") {
				host_header := strings.TrimSpace(header[5:])
				host_parts := strings.SplitN(host_header, ":", 2)
				if len(host_parts) < 1 {
					return
				}
				host = fmt.Sprintf("%s:%s", host_parts[0], httpsep_parts[1])
			}
			if len(strings.TrimSpace(header)) == 0 {
				writer := bufio.NewWriter(conn)
				writer.WriteString("HTTP/1.1 301 Moved Permanently\r\n")
				location_header := fmt.Sprintf("Location: https://%s%s\r\n", host, path)
				writer.WriteString(location_header)
				writer.WriteString("Content-Type: text/html; charset=utf-8\r\n")
				writer.WriteString("Content-Length: 0\r\n")
				writer.WriteString("\r\n")
				writer.Flush()
				return
			}
		}
	}
	setupConn80 := func(node tree.Node, conn net.Conn, id Id) {
		defer node.IfRecoverCloser(conn.Close)
		addr := conn.RemoteAddr().String()
		cid := id.Next(addr)
		child := node.AddChild(cid)
		child.AddCloser("conn", conn.Close)
		child.AddProcess("handler", func() {
			log.Println("open80", count80.increment(), cid)
			defer func() { log.Println("close80", count80.decrement(), cid) }()
			handleConn80(child, conn)
		})
	}
	node.AddProcess("server80", func() {
		id := NewId("proxy-" + listen80.Addr().String())
		for {
			conn80, err := listen80.Accept()
			if err != nil {
				log.Println(err)
				break
			}
			setupConn80(node, conn80, id)
		}
	})
}

func readLine(conn net.Conn, maxlen int) (string, error) {
	bb := bytes.Buffer{}
	b := []byte{0}
	for {
		n, err := conn.Read(b)
		if err == nil && n != 1 {
			err = fmt.Errorf("invalid read count %d", n)
		}
		if err != nil {
			return bb.String(), err
		}
		n, err = bb.Write(b)
		if err == nil && n != 1 {
			err = fmt.Errorf("invalid write count %d", n)
		}
		if err != nil {
			return bb.String(), err
		}
		if b[0] == '\n' {
			return bb.String(), nil
		}
		if bb.Len() >= maxlen {
			err = fmt.Errorf("line too long > %d", maxlen)
			return bb.String(), err
		}
	}
}

type StateDro struct {
	Sid  string
	Port int
	Ship string
	Wts  time.Time
	Host string
	IP   string
}

func dockProxy(target *url.URL, ship *StateDro) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Path = target.Path
		req.URL.Host = ship.Ship
	}
	return &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			// MaxIdleConns:        1,
			// MaxConnsPerHost:     1,
			// MaxIdleConnsPerHost: 1,
			IdleConnTimeout: 1 * time.Second,
			//DialContext has precedence over Dial, both left for completenes shake
			DialContext: func(parent context.Context, network, addr string) (net.Conn, error) {
				listen := fmt.Sprintf("%s:%d", ship.IP, ship.Port)
				ctx, cancel := context.WithTimeout(parent, 5*time.Second)
				defer cancel()
				var dialer = &net.Dialer{}
				conn, err := dialer.DialContext(ctx, "tcp", listen)
				if err != nil {
					return nil, err
				}
				header := fmt.Sprintf("%s\n", target.Host)
				n, err := conn.Write([]byte(header))
				if err == nil && n != len(header) {
					err = fmt.Errorf("write mismatch %d %d", len(header), n)
				}
				if err != nil {
					conn.Close()
					return nil, err
				}
				keepAlive(conn)
				return conn, nil
			},
			Dial: func(network, addr string) (net.Conn, error) {
				listen := fmt.Sprintf("%s:%d", ship.IP, ship.Port)
				conn, err := net.DialTimeout("tcp", listen, 5*time.Second)
				if err != nil {
					return nil, err
				}
				header := fmt.Sprintf("%s\n", target.Host)
				n, err := conn.Write([]byte(header))
				if err == nil && n != len(header) {
					err = fmt.Errorf("write mismatch %d %d", len(header), n)
				}
				if err != nil {
					conn.Close()
					return nil, err
				}
				keepAlive(conn)
				return conn, nil
			},
		},
	}
}

type server443Dso struct {
	mainReverse *httputil.ReverseProxy
	mainURL     *url.URL
	dockep      string
	dao         Dao
}

func (dso *server443Dso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/proxy/") {
		// /proxy/SHIP/PATH
		target_ep_path := r.URL.Path[len("/proxy/"):]
		if !strings.Contains(target_ep_path, "/") {
			target_ep_path += "/"
		}
		target_parts := strings.SplitN(target_ep_path, "/", 2)
		target_ship := target_parts[0]
		target_path := "/" + target_parts[1]
		if strings.Contains(target_ship, ":") {
			http.Error(w, "port not supported", 400)
			return
		}
		ship_record, err := dso.dao.GetShip(target_ship)
		if err != nil {
			http.Error(w, "ship not found", 400)
			return
		}
		if !ship_record.Enabled {
			http.Error(w, "ship disabled", 400)
			return
		}
		http_client := &http.Client{}
		state_urlf := "http://%s/api/ship/state/%s"
		state_url := fmt.Sprintf(state_urlf, dso.dockep, ship_record.Ship)
		state_resp, err := http_client.Get(state_url)
		if err != nil {
			http.Error(w, "state get error", 400)
			return
		}
		state_body, err := io.ReadAll(state_resp.Body)
		if err != nil {
			http.Error(w, "state read error", 400)
			return
		}
		ship_state := &StateDro{}
		err = json.Unmarshal(state_body, ship_state)
		if err != nil {
			http.Error(w, "state parse error", 400)
			return
		}
		if ship_state.Port < 0 {
			http.Error(w, "ship disabled", 400)
			return
		}
		target_urls := fmt.Sprintf("%s%s", ship_record.Prefix, target_path)
		target_url, err := url.Parse(target_urls)
		if err != nil {
			http.Error(w, "url parse error", 400)
			return
		}
		// log.Println("target_ep_path", target_ep_path)
		// log.Println("target_path", target_path)
		// log.Println("target_urls", target_urls)
		dock_proxy := dockProxy(target_url, ship_state)
		dock_proxy.ServeHTTP(w, r)
	} else {
		r.Host = dso.mainURL.Host
		dso.mainReverse.ServeHTTP(w, r)
	}
}

type server80Dso struct {
	hostname string
}

func (dso *server80Dso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("https://%s%s", dso.hostname, r.URL.Path)
	http.Redirect(w, r, url, http.StatusMovedPermanently)
}
