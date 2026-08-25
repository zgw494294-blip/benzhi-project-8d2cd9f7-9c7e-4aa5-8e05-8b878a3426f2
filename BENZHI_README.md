# BENZHI_README

基于 Go 实现的生物样本冷链接收放行台 Web 项目，一款后端服务，生物样本冷链接收放行台提供案卷创建、容器与探头登记、运输证据采集、风险复算、质量审核退回补正、复审放行和凭据完整性核验的同源浏览器工作台。

## 项目说明
- 项目：benzhi-project-8d2cd9f7-9c7e-4aa5-8e05-8b878a3426f2
- 项目用途：生物样本冷链接收放行台提供案卷创建、容器与探头登记、运输证据采集、风险复算、质量审核退回补正、复审放行和凭据完整性核验的同源浏览器工作台。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/coldchain-server -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-8d2cd9f7-9c7e-4aa5-8e05-8b878a3426f2-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-8d2cd9f7-9c7e-4aa5-8e05-8b878a3426f2-arm64 linux/arm64
docker run -it benzhi-project-8d2cd9f7-9c7e-4aa5-8e05-8b878a3426f2-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/coldchain-server -selfcheck -addr=127.0.0.1:19081`
