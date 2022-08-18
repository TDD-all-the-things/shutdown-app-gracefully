# Shutdown APP Gracefully

# 运行测试

1. 运行全部测试

```shell
$ cd shutdown-app-gracefully
$ go test -cover -count=1 ./... -timeout=10s

?       github.com/TDD-all-the-things/shutdown-app-gracefully   [no test files]
ok      github.com/TDD-all-the-things/shutdown-app-gracefully/app       0.706s  coverage: 0.0% of statements
ok      github.com/TDD-all-the-things/shutdown-app-gracefully/server    0.572s  coverage: 100.0% of statements
```

2. 查看app包中生成的覆盖率文件

app包中运行在子进程中的测试用例集中只有正常退出的测试用例才会生成测试覆盖率报告.

```shell
$ cd shutdown-app-gracefully
$ go tool cover -html ./app/TestApp_Exit_Gracefully.out
```

# 运行代码

```shell
$ cd shutdown-app-gracefully
$ go build
$ ./shutdown-app-gracefully
$ ctrl+c/ctrl+c
```

# Todo

- 处理app中所有放出的Goroutine的panic/error