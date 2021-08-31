package proxy

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rb3ckers/trafficmirror/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestReflector(t *testing.T) {
	reqs1, reqs2 := 0, 0
	wg := &sync.WaitGroup{}
	wg.Add(2)
	serv1 := gin.New()
	serv2 := gin.New()

	serv1.GET("/", func(c *gin.Context) {
		reqs1++
		wg.Done()
		c.String(200, "Hello World")
	})

	serv2.GET("/", func(c *gin.Context) {
		reqs2++
		wg.Done()
		c.String(200, "Hello World")
	})

	go gin.Default().Run(":8888")
	go serv1.Run(":8081")
	go serv2.Run(":8082")

	p := NewProxy(config.Default())
	assert.NoError(t, p.Start(context.Background()))
	p.reflector.AddMirrors([]string{"http://localhost:8081", "http://localhost:8082"})

	req, err := http.NewRequest("GET", "http://localhost:8080/", nil)
	assert.NoError(t, err)

	c := &http.Client{
		Timeout: time.Second * 20,
	}

	resp, err := c.Do(req)
	assert.NoError(t, err)
	resp.Body.Close()

	wg.Wait()

	assert.Equal(t, 1, reqs1)
	assert.Equal(t, 1, reqs2)
}
