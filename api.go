package main

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samuelventura/go-tree"
)

func api(node tree.Node) {
	dao := node.GetValue("dao").(Dao)
	endpoint := node.GetValue("endpoint").(string)
	gin.SetMode(gin.ReleaseMode) //remove debug warning
	router := gin.New()          //remove default logger
	router.Use(gin.Recovery())   //looks important
	skapi := router.Group("/api/ship")
	skapi.GET("/count", func(c *gin.Context) {
		count := dao.CountShips()
		c.JSON(200, count)
	})
	skapi.GET("/count/enabled", func(c *gin.Context) {
		count := dao.CountEnabledShips()
		c.JSON(200, count)
	})
	skapi.GET("/count/disabled", func(c *gin.Context) {
		count := dao.CountDisabledShips()
		c.JSON(200, count)
	})
	skapi.GET("/info/:name", func(c *gin.Context) {
		name := c.Param("name")
		row, err := dao.GetShip(name)
		if err != nil {
			c.JSON(400, fmt.Sprintf("err: %v", err))
			return
		}
		c.JSON(200, row)
	})
	skapi.POST("/add/:name", func(c *gin.Context) {
		name := c.Param("name")
		prefix, ok := c.GetQuery("prefix")
		if !ok {
			c.JSON(400, "err: missing prefix param")
			return
		}
		err := dao.AddShip(name, prefix)
		if err != nil {
			c.JSON(400, fmt.Sprintf("err: %v", err))
			return
		}
		c.JSON(200, "ok")
	})
	skapi.POST("/enable/:name", func(c *gin.Context) {
		name := c.Param("name")
		err := dao.EnableShip(name, true)
		if err != nil {
			c.JSON(400, fmt.Sprintf("err: %v", err))
			return
		}
		c.JSON(200, "ok")
	})
	skapi.POST("/disable/:name", func(c *gin.Context) {
		name := c.Param("name")
		err := dao.EnableShip(name, false)
		if err != nil {
			c.JSON(400, fmt.Sprintf("err: %v", err))
			return
		}
		c.JSON(200, "ok")
	})
	listen, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Fatal(err)
	}
	node.AddCloser("listen", listen.Close)
	port := listen.Addr().(*net.TCPAddr).Port
	log.Println("port api", port)
	server := &http.Server{
		Addr:    endpoint,
		Handler: router,
	}
	node.AddProcess("server", func() {
		err = server.Serve(listen)
		if err != nil {
			log.Println(endpoint, port, err)
		}
	})
}
