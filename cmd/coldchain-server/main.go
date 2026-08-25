package main

import (
	"coldchain/internal/application"
	"coldchain/internal/httpapi"
	"coldchain/internal/storage"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func listenAddr(flagValue string) (string, error) {
	a := flagValue
	if a == "" {
		if p := os.Getenv("PORT"); p != "" {
			n, e := strconv.Atoi(p)
			if e != nil || n < 1024 || n > 65535 {
				return "", fmt.Errorf("PORT 无效")
			}
			a = "127.0.0.1:" + p
		} else {
			a = "127.0.0.1:19081"
		}
	}
	if !strings.HasPrefix(a, "127.0.0.1:") {
		return "", fmt.Errorf("监听地址必须为回环地址")
	}
	return a, nil
}
func main() {
	addrFlag := flag.String("addr", "", "监听地址")
	self := flag.Bool("selfcheck", false, "执行完整自检")
	data := flag.String("data", "./data", "本地数据目录")
	flag.Parse()
	addr, e := listenAddr(*addrFlag)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	st, e := storage.New(*data)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	app := application.New(st)
	srv := httpapi.New(app)
	server := &http.Server{Addr: addr, Handler: srv.Handler()}
	if *self {
		if e := runSelfcheck(server, addr); e != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", e)
			os.Exit(1)
		}
		return
	}
	go func() {
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, e)
		}
	}()
	waitForSignal(server)
}
func waitForSignal(server *http.Server) {
	ch := make(chan os.Signal, 1)
	signalNotify(ch)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
func signalNotify(ch chan<- os.Signal) { signal.Notify(ch, os.Interrupt, syscall.SIGTERM) }
func runSelfcheck(server *http.Server, addr string) error {
	go server.ListenAndServe()
	client := &http.Client{Timeout: 3 * time.Second}
	base := "http://" + addr
	var start = time.Now()
	for time.Since(start) < 2*time.Second {
		r, e := client.Get(base + "/")
		if e == nil {
			r.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	now := time.Now().UTC()
	post := func(path string, v any) (map[string]any, error) {
		b, _ := json.Marshal(v)
		r, e := client.Post(base+path, "application/json", strings.NewReader(string(b)))
		if e != nil {
			return nil, e
		}
		defer r.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(r.Body).Decode(&out)
		if r.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %v", path, out)
		}
		return out, nil
	}
	caseNumber := fmt.Sprintf("SC-SELF-%d", now.UnixNano())
	c, e := post("/api/cases", map[string]any{"caseNumber": caseNumber, "senderName": "发送方", "receiverName": "接收方", "handoffWindowStart": now.Format(time.RFC3339), "handoffWindowEnd": now.Add(6 * time.Hour).Format(time.RFC3339)})
	if e != nil {
		return e
	}
	id := c["id"].(string)
	version := int(c["version"].(float64))
	c, e = post("/api/cases/"+id+"/containers", map[string]any{"expectedVersion": version, "id": "box", "containerCode": "BOX-S", "sealCode": "SEAL-S", "minTemperatureC": 2, "maxTemperatureC": 8})
	if e != nil {
		return e
	}
	version = int(c["version"].(float64))
	c, e = post("/api/cases/"+id+"/probes", map[string]any{"expectedVersion": version, "id": "probe", "serialNumber": "PROBE-S", "certificateRef": "CERT-S", "calibratedAt": now.Add(-time.Hour).Format(time.RFC3339), "calibrationExpiresAt": now.Add(7 * time.Hour).Format(time.RFC3339), "accuracyC": 0.2})
	if e != nil {
		return e
	}
	version = int(c["version"].(float64))
	c, e = post("/api/cases/"+id+"/evidence", map[string]any{"expectedVersion": version, "probeId": "probe", "segmentStart": now.Format(time.RFC3339), "segmentEnd": now.Add(6 * time.Hour).Format(time.RFC3339), "readings": []map[string]any{{"at": now.Format(time.RFC3339), "temperatureC": 4}, {"at": now.Add(3 * time.Hour).Format(time.RFC3339), "temperatureC": 4}, {"at": now.Add(6 * time.Hour).Format(time.RFC3339), "temperatureC": 4}}, "sealObservation": "SEAL-S"})
	if e != nil {
		return e
	}
	version = int(c["version"].(float64))
	c, e = post("/api/cases/"+id+"/submit", map[string]any{"expectedVersion": version})
	if e != nil {
		return e
	}
	version = int(c["version"].(float64))
	if raw, ok := c["findings"].([]any); ok {
		for _, f := range raw {
			m := f.(map[string]any)
			c, e = post("/api/cases/"+id+"/decisions", map[string]any{"expectedVersion": version, "findingID": m["id"], "decision": "接受", "note": "审核通过", "reviewer": "审核员"})
			if e != nil {
				return e
			}
			version = int(c["version"].(float64))
		}
	}
	c, e = post("/api/cases/"+id+"/approve", map[string]any{"expectedVersion": version, "reviewer": "审核员"})
	if e != nil {
		return e
	}
	version = int(c["version"].(float64))
	cr, e := post("/api/cases/"+id+"/release", map[string]any{"expectedVersion": version, "reviewer": "审核员"})
	if e != nil {
		return e
	}
	if cr["credentialNumber"] == nil {
		return fmt.Errorf("凭据缺失")
	}
	_, e = post("/api/cases/"+id+"/containers", map[string]any{"expectedVersion": version, "id": "bad", "containerCode": "BAD", "sealCode": "BAD", "minTemperatureC": 8, "maxTemperatureC": 2})
	if e == nil {
		return fmt.Errorf("失败请求未被拒绝")
	}
	_, _ = io.Copy(io.Discard, strings.NewReader(""))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
