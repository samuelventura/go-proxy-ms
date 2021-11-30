package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/samuelventura/go-tools"
	"github.com/samuelventura/go-tree"
)

type StateDro struct {
	Sid  string
	Port int
	Ship string
	Wts  time.Time
	Host string
	IP   string
}

func entry(node tree.Node) {
	dao := node.GetValue("dao").(Dao)
	crtPath := node.GetValue("server.crt").(string)
	keyPath := node.GetValue("server.key").(string)
	//hostname := node.GetValue("hostname").(string)
	http_ep := node.GetValue("http").(string)
	https_ep := node.GetValue("https").(string)
	dock_ep := node.GetValue("dock").(string)
	https_parts := strings.SplitN(https_ep, ":", 2)
	if len(https_parts) != 2 {
		log.Panicf("https ep must have 2 parts")
	}
	main, err := url.Parse(node.GetValue("main").(string))
	tools.PanicIfError(err)
	cer, err := tls.LoadX509KeyPair(crtPath, keyPath)
	tools.PanicIfError(err)
	count443 := newCount()
	tlscs := &tls.Config{Certificates: []tls.Certificate{cer}}
	listen443, err := tls.Listen("tcp", https_ep, tlscs)
	tools.PanicIfError(err)
	node.AddCloser("listen443", listen443.Close)
	http_client := &http.Client{}
	handleConn443 := func(node tree.Node, connIn net.Conn) {
		err := connIn.SetDeadline(time.Now().Add(4 * time.Second))
		tools.PanicIfError(err)
		head, err := readLine(connIn, 256)
		if err != nil {
			return
		}
		head_parts := strings.SplitN(head, " ", 3)
		if len(head_parts) != 3 {
			return
		}
		err = connIn.SetDeadline(time.Time{})
		tools.PanicIfError(err)
		path := head_parts[1]
		var connOut net.Conn
		var pathRewrite bool
		var pathPrefix string
		if strings.HasPrefix(path, "/proxy/") {
			// /proxy/SHIP/PATH
			target_ep_path := path[len("/proxy/"):]
			if !strings.Contains(target_ep_path, "/") {
				target_ep_path += "/"
			}
			target_parts := strings.SplitN(target_ep_path, "/", 2)
			target_ship := target_parts[0]
			target_path := "/" + target_parts[1]
			if strings.Contains(target_ship, ":") {
				log.Println("port not supported")
				return
			}
			ship_record, err := dao.GetShip(target_ship)
			if err != nil {
				log.Println("ship not found")
				return
			}
			if !ship_record.Enabled {
				log.Println("ship disabled")
				return
			}
			state_urlf := "http://%s/api/ship/state/%s"
			state_url := fmt.Sprintf(state_urlf, dock_ep, ship_record.Ship)
			state_resp, err := http_client.Get(state_url)
			if err != nil {
				log.Println("state get error")
				return
			}
			state_body, err := io.ReadAll(state_resp.Body)
			if err != nil {
				log.Println("state read error")
				return
			}
			ship_state := &StateDro{}
			err = json.Unmarshal(state_body, ship_state)
			if err != nil {
				log.Println("state parse error")
				return
			}
			if ship_state.Port <= 0 {
				log.Println("ship disabled")
				return
			}
			target_urls := fmt.Sprintf("%s%s", ship_record.Prefix, target_path)
			target_url, err := url.Parse(target_urls)
			if err != nil {
				log.Println("url parse error")
				return
			}
			proxy_ep := fmt.Sprintf("%s:%d", ship_state.IP, ship_state.Port)
			connOut, err = dialProxy(proxy_ep, target_url.Scheme, target_url.Host)
			if err != nil {
				log.Println(err)
				return
			}
			node.AddCloser("connOut", connOut.Close)
			head = fmt.Sprintf("%s %s %s", head_parts[0], target_path, head_parts[2])
			pathPrefix = fmt.Sprintf("/proxy/%s", target_ship)
			pathRewrite = true
		} else {
			connOut, err = dialMain(main.Scheme, main.Host)
			if err != nil {
				log.Println(err)
				return
			}
			node.AddCloser("connOut", connOut.Close)
		}
		head_bytes := []byte(head)
		n, err := connOut.Write(head_bytes)
		if err != nil {
			log.Println(err)
			return
		}
		if n != len(head_bytes) {
			log.Println("len mismatch", n)
			return
		}
		node.AddProcess("copy(connIn, connOut)", func() {
			io.Copy(connIn, connOut)
		})
		if pathRewrite {
			node.AddProcess("copy(connOut, connIn)", func() {
				//will only check first bytes of each packet
				//assuming the response introduces a noticeable break
				data := make([]byte, 2048)
				buf := bytes.Buffer{}
				for {
					rn, err := connIn.Read(data)
					if err != nil {
						return
					}
					buf.Reset()
					wdata := data[:rn]
					for i := 0; i < 64 && i < rn; i++ {
						d := data[i]
						buf.WriteByte(d)
						if d == '\n' {
							line := buf.String()
							if strings.Contains(line, "HTTP/1.1") {
								line_parts := strings.SplitN(line, " ", 3)
								if len(line_parts) == 3 {
									if strings.HasPrefix(line_parts[2], "HTTP/1.1") {
										if strings.HasPrefix(line_parts[1], pathPrefix) {
											index := strings.Index(line, pathPrefix)
											if index > 0 {
												plen := len(pathPrefix)
												for j := 0; j < index; j++ {
													data[j+plen] = data[j]
												}
												wdata = data[index:]
												//log.Println(line)
											}
										}
									}
								}
							}
							break
						}
					}
					wn, err := connOut.Write(wdata)
					if err != nil {
						return
					}
					if wn != len(wdata) {
						return
					}
				}
			})
		} else {
			node.AddProcess("copy(connOut, connIn)", func() {
				io.Copy(connOut, connIn)
			})
		}
		node.WaitClosed()
	}
	setupConn443 := func(node tree.Node, conn net.Conn, id Id) {
		defer node.IfRecoverCloser(conn.Close)
		addr := conn.RemoteAddr().String()
		cid := id.Next(addr)
		child := node.AddChild(cid)
		child.AddCloser("connIn", conn.Close)
		child.AddProcess("handler", func() {
			log.Println("open443", count443.increment(), cid)
			defer func() { log.Println("close443", count443.decrement(), cid) }()
			handleConn443(child, conn)
		})
	}
	node.AddProcess("server443", func() {
		id := NewId("proxy443-" + listen443.Addr().String())
		for {
			conn443, err := listen443.Accept()
			if err != nil {
				log.Println(err)
				break
			}
			setupConn443(node, conn443, id)
		}
	})
	count80 := newCount()
	listen80, err := net.Listen("tcp", http_ep)
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
				host = fmt.Sprintf("%s:%s", host_parts[0], https_parts[1])
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
		id := NewId("proxy80-" + listen80.Addr().String())
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

func dialMain(scheme, host string) (net.Conn, error) {
	switch scheme {
	case "http":
		addr := host
		if !strings.Contains(host, ":") {
			addr += ":80"
		}
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case "https":
		addr := host
		if !strings.Contains(host, ":") {
			addr += ":443"
		}
		tlscc := &tls.Config{InsecureSkipVerify: true}
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlscc)
		if err != nil {
			return nil, err
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("unknown scheme %s", scheme)
	}
}

func dialProxy(proxy_ep, scheme, host string) (net.Conn, error) {
	addr := host
	switch scheme {
	case "http":
		if !strings.Contains(host, ":") {
			addr += ":80"
		}
	case "https":
		if !strings.Contains(host, ":") {
			addr += ":443"
		}
	default:
		return nil, fmt.Errorf("unknown scheme %s", scheme)
	}
	conn, err := net.DialTimeout("tcp", proxy_ep, 5*time.Second)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("%s\n", addr)
	n, err := conn.Write([]byte(header))
	if err == nil && n != len(header) {
		err = fmt.Errorf("write mismatch %d %d", len(header), n)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	if scheme == "https" {
		tlscc := &tls.Config{InsecureSkipVerify: true}
		conns := tls.Client(conn, tlscc)
		err := conns.Handshake()
		if err != nil {
			conn.Close()
			return nil, err
		}
		return conns, nil
	}
	return conn, nil
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
