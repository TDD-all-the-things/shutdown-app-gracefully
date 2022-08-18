package server_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/TDD-all-the-things/shutdown-app-gracefully/app"
	"github.com/TDD-all-the-things/shutdown-app-gracefully/server"

	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {

	name, addr, URI, message := "test", "localhost:9090", "/", "OK"
	var srv app.Server
	srv = server.New(name, addr)

	assert.Equal(t, name, srv.Name())
	assert.Equal(t, addr, srv.Addr())

	srv.Handle(URI, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(message))
	}))

	errChan := make(chan error)

	// 启动服务器
	go func() {
		if err := srv.Start(); err != nil {
			errChan <- err
		}
	}()

	select {
	case <-errChan:
		t.Error("Failed to start server")
	case <-time.After(10 * time.Millisecond):
		// 等待Server正常运行
		break
	}

	// 创建TCP连接
	conn, err := net.Dial("tcp", addr)
	assert.NoError(t, err)
	defer conn.Close()

	// 构造HTTP Request
	request, err := http.NewRequest(http.MethodGet, "http://"+addr+URI, nil)
	assert.NoError(t, err)

	t.Run("should handle request if server is running", func(t *testing.T) {

		// 使用其中一个TCP连接 发送HTTP Request
		request.Write(conn)
		// 读取对应的Response
		resp, err := http.ReadResponse(bufio.NewReader(conn), request)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// 对Response Body中的内容进行断言
		msg, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, message, string(msg))
	})

	t.Run("should reject request if server is stopping", func(t *testing.T) {

		ch := make(chan *http.Response)

		// 再创建一个TCP连接
		conn2, err := net.Dial("tcp", addr)
		assert.NoError(t, err)
		defer conn.Close()

		// 关闭服务器
		go func() {
			if err := srv.Stop(context.Background()); err != nil {
				errChan <- err
			}
		}()

		// 用两个TCP连接并发请求,增大请求成功概率,使代码执行流程执行到reject request
		go func() {
			request.Write(conn)
			resp, err := http.ReadResponse(bufio.NewReader(conn), request)
			if err == nil {
				ch <- resp
			}
		}()

		go func() {
			request.Write(conn2)
			resp, err := http.ReadResponse(bufio.NewReader(conn2), request)
			if err == nil {
				ch <- resp
			}
		}()

		resp := <-ch
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Contains(t, resp.Status, http.StatusText(http.StatusServiceUnavailable))
	})
}
