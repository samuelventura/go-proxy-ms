package main

import (
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

	"github.com/samuelventura/go-tree"
)

func entry(node tree.Node) {
	dao := node.GetValue("dao").(Dao)
	crt := node.GetValue("server.crt").(string)
	key := node.GetValue("server.key").(string)
	hostname := node.GetValue("hostname").(string)
	httpep := node.GetValue("http").(string)
	httpsep := node.GetValue("https").(string)
	dockep := node.GetValue("dock").(string)
	mainulrs := node.GetValue("main").(string)
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
		Handler: &server443Dso{mainrp, mainulr, dockep, dao},
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
		Handler: &server80Dso{hostname},
	}
	node.AddProcess("server80", func() {
		err = server80.Serve(listen80)
		if err != nil {
			log.Println(httpep, err)
		}
	})
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
