# 本质评测环境说明

## 项目

- 项目编号：`hwj-gowork-110`
- 项目名称：高纬度天文台低温仪器观测窗口与校准归档服务
- 项目说明：基于 Go HTTP JSON 与 SQLite 文件持久化，管理天文台仪器、低温预冷、校准方案、观测窗口、质量复测和数据归档。

## 固定环境

- Go toolchain：`go1.26.5`
- go.mod language version：`go 1.21`
- GOTOOLCHAIN：`local`
- 支持平台：`linux/amd64`、`linux/arm64`
- Docker 基础镜像：`golang:1.26.5-bookworm`
- Docker manifest：`golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd`

## 构建

评测镜像使用仓库内固定的 `benzhi.Dockerfile`，并通过 `build_benzhi_docker.sh` 构建：

```bash
./build_benzhi_docker.sh hwj-gowork-110:benzhi-amd64 linux/amd64
./build_benzhi_docker.sh hwj-gowork-110:benzhi-arm64 linux/arm64
```

## 运行

```bash
docker run --rm -it --network none hwj-gowork-110:benzhi-amd64 bash
```

## 容器内验证

```bash
go version
go env GOTOOLCHAIN GOPROXY GOMODCACHE GOCACHE
go test ./...
go vet ./...
go build ./...
```
