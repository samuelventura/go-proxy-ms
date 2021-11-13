package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/signal"

	"github.com/samuelventura/go-state"
	"github.com/samuelventura/go-tree"
)

func main() {
	os.Setenv("GOTRACEBACK", "all")
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	ctrlc := make(chan os.Signal, 1)
	signal.Notify(ctrlc, os.Interrupt)

	log.Println("start", os.Getpid())
	defer log.Println("exit")

	rnode := tree.NewRoot("root", log.Println)
	defer rnode.WaitDisposed()
	//recover closes as well
	defer rnode.Recover()

	spath := state.SingletonPath()
	snode := state.Serve(rnode, spath)
	defer snode.WaitDisposed()
	defer snode.Close()
	log.Println("socket", spath)

	enode := rnode.AddChild("entry")
	defer enode.WaitDisposed()
	defer enode.Close()
	enode.SetValue("hostname", getenv("PROXY_HOSTNAME", hostname()))
	enode.SetValue("http", getenv("PROXY_HTTP_EP", ":80"))
	enode.SetValue("https", getenv("PROXY_HTTPS_EP", ":443"))
	enode.SetValue("dock", getenv("PROXY_DOCK_EP", "127.0.0.1:31623"))
	enode.SetValue("main", getenv("PROXY_MAIN_EP", "127.0.0.1:8080"))
	enode.SetValue("server.crt", getenv("PROXY_SERVER_CRT", withext("crt")))
	enode.SetValue("server.key", getenv("PROXY_SERVER_KEY", withext("key")))
	entry(enode)

	stdin := make(chan interface{})
	go func() {
		defer close(stdin)
		ioutil.ReadAll(os.Stdin)
	}()
	select {
	case <-rnode.Closed():
	case <-snode.Closed():
	case <-enode.Closed():
	case <-ctrlc:
	case <-stdin:
	}
}
