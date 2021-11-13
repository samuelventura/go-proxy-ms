package main

import (
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

	"github.com/samuelventura/go-tree"
)

func entry(node tree.Node) {
	crt := node.GetValue("server.crt").(string)
	key := node.GetValue("server.key").(string)
	httpep := node.GetValue("http").(string)
	httpsep := node.GetValue("https").(string)
	dockep := node.GetValue("dock").(string)
	mainep := node.GetValue("main").(string)
	mainulrs := fmt.Sprintf("http://%s/", mainep)
	mainulr, err := url.Parse(mainulrs)
	if err != nil {
		log.Fatal(err)
	}
	mainrp := httputil.NewSingleHostReverseProxy(mainulr)
	listen443, err := net.Listen("tcp", httpsep)
	if err != nil {
		log.Fatal(err)
	}
	node.AddCloser("listen443", listen443.Close)
	server443 := &http.Server{
		Addr:    httpsep,
		Handler: &server443Dso{mainrp, dockep},
	}
	node.AddProcess("server443", func() {
		err = server443.ServeTLS(listen443, crt, key)
		if err != nil {
			log.Println(httpsep, err)
		}
	})
	listen80, err := net.Listen("tcp", httpep)
	if err != nil {
		log.Fatal(err)
	}
	node.AddCloser("listen80", listen80.Close)
	server80 := &http.Server{
		Addr:    httpep,
		Handler: &server80Dso{},
	}
	node.AddProcess("server80", func() {
		err = server80.Serve(listen80)
		if err != nil {
			log.Println(httpep, err)
		}
	})
}

type shipStatus struct {
	ip   string
	port int64
}

func dockProxy(proxy, scheme, host, path string) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = scheme
		req.URL.Host = host
		req.URL.Path = path
	}
	return &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				client := &http.Client{}
				// SHIPNAME:PORT
				parts := strings.SplitN(addr, ":", 2)
				url := fmt.Sprintf("http://%s/api/ship/status/%s", proxy, parts[0])
				resp, err := client.Get(url)
				if err != nil {
					return nil, err
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, err
				}
				ship := &shipStatus{}
				err = json.Unmarshal([]byte(body), &ship)
				if err != nil {
					return nil, err
				}
				if ship.port < 0 {
					err = fmt.Errorf("not available: %s", parts[0])
					return nil, err
				}
				listen := fmt.Sprintf("%s:%d", ship.ip, ship.port)
				conn, err := net.DialTimeout("tcp", listen, 5*time.Second)
				if err != nil {
					return nil, err
				}
				line := fmt.Sprintf("127.0.0.1:%s\n", parts[1])
				n, err := conn.Write([]byte(line))
				if err == nil && n != len(line) {
					err = fmt.Errorf("write mismatch %d %d", len(line), n)
				}
				if err != nil {
					conn.Close()
					return nil, err
				}
				keepAlive(conn)
				return conn, nil
			},
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
}

type server443Dso struct {
	main  *httputil.ReverseProxy
	proxy string
}

func (dso *server443Dso) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if strings.HasPrefix(r.URL.Path, "/proxy/") {
		target := r.URL.Path[len("/proxy/"):]
		// /SHIPNAME:PORT/PATH
		parts := strings.SplitN(target, "/", 2)
		proxy := dockProxy(dso.proxy, r.URL.Scheme, parts[0], parts[1])
		proxy.ServeHTTP(w, r)
	} else {
		dso.main.ServeHTTP(w, r)
	}
}

type server80Dso struct{}

func (dso *server80Dso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Scheme {
	case "http":
		r.URL.Scheme = "https"
	case "ws":
		r.URL.Scheme = "wss"
	default:
		err := fmt.Sprintf("Unsupported schema: %s", r.URL)
		http.Error(w, err, 400)
		return
	}
	http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
}
