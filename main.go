package main

import (
	"log"
	"os"

	"github.com/samuelventura/go-state"
	"github.com/samuelventura/go-tools"
	"github.com/samuelventura/go-tree"
)

func main() {
	tools.SetupLog()

	ctrlc := tools.SetupCtrlc()
	stdin := tools.SetupStdinAll()

	log.Println("start", os.Getpid())
	defer log.Println("exit")

	rnode := tree.NewRoot("root", log.Println)
	defer rnode.WaitDisposed()
	//recover closes as well
	defer rnode.Recover()
	rnode.SetValue("driver", tools.GetEnviron("PROXY_DB_DRIVER", "sqlite"))
	rnode.SetValue("source", tools.GetEnviron("PROXY_DB_SOURCE", tools.WithExtension("db3")))
	rnode.SetValue("state", tools.GetEnviron("PROXY_STATE", tools.WithExtension("state")))
	dao := NewDao(rnode) //close on root
	rnode.AddCloser("dao", dao.Close)
	rnode.SetValue("dao", dao)

	snode := state.Serve(rnode, rnode.GetValue("state").(string))
	defer snode.WaitDisposed()
	defer snode.Close()

	enode := rnode.AddChild("entry")
	defer enode.WaitDisposed()
	defer enode.Close()
	enode.SetValue("hostname", tools.GetEnviron("PROXY_HOSTNAME", tools.GetHostname()))
	enode.SetValue("http", tools.GetEnviron("PROXY_HTTP_EP", ":80"))
	enode.SetValue("https", tools.GetEnviron("PROXY_HTTPS_EP", ":443"))
	enode.SetValue("dock", tools.GetEnviron("PROXY_DOCK_EP", "127.0.0.1:31623"))
	enode.SetValue("main", tools.GetEnviron("PROXY_MAIN_URL", "http://127.0.0.1:8080"))
	enode.SetValue("server.crt", tools.GetEnviron("PROXY_SERVER_CRT", tools.WithExtension("crt")))
	enode.SetValue("server.key", tools.GetEnviron("PROXY_SERVER_KEY", tools.WithExtension("key")))
	entry(enode)

	anode := rnode.AddChild("api")
	defer anode.WaitDisposed()
	defer anode.Close()
	anode.SetValue("endpoint", tools.GetEnviron("PROXY_API_EP", "127.0.0.1:31688"))
	api(anode)

	select {
	case <-rnode.Closed():
	case <-snode.Closed():
	case <-enode.Closed():
	case <-anode.Closed():
	case <-ctrlc:
	case <-stdin:
	}
}
