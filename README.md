##

```sh
docker run --rm -ti --name wg2 --privileged --net host shynome/libp2p-wg2 -key 1
```

## 问题

- [outbound.go](./endpoint/outbound.go#L87) libp2p stream 连接失败, [bind.go](./bind.go#L80) 这里是 libp2p host 初始化

## 复现

```sh
# 启动节点A, 等待通告路由后再执行下一步
make run
# 在 VS Code 里按 F5 运行程序
# 会一直看到连接失败的信息
```
