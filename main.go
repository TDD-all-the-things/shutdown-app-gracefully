package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/TDD-all-the-things/shutdown-app-gracefully/app"
	"github.com/TDD-all-the-things/shutdown-app-gracefully/server"
)

func main() {
	s1 := server.New("business", "localhost:8080")
	s1.Handle("/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("hello"))
	}))
	s2 := server.New("admin", "localhost:8081")
	a, err := app.New([]app.Server{s1, s2},
		app.WithShutdownCallbacks(StoreCacheToDBCallback),
		app.WithShutdownTimeout(10*time.Second),
		app.WithWaitTimeout(5*time.Second),
		app.WithCallbackTimeout(3*time.Second))
	if err != nil {
		panic(err)
	}
	a.StartAndServe()
}

func StoreCacheToDBCallback(ctx context.Context) {
	done := make(chan struct{}, 1)
	go func() {
		// 你的业务逻辑，比如说这里我们模拟的是将本地缓存刷新到数据库里面
		// 这里我们简单的睡一段时间来模拟
		log.Println("刷新缓存中……")
		time.Sleep(1 * time.Second)
		done <- struct{}{}
	}()
	select {
	case <-ctx.Done():
		log.Println("刷新缓存超时")
	case <-done:
		log.Println("缓存被刷新到了 DB")
	}
}
