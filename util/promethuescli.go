package util

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

var PrometheusClient = InitClient()
var NodeIP = map[string]string{
	"node1": "116.56.140.23",
	"node2": "116.56.140.108",
	"node3": "116.56.140.131",
}

func InitClient() api.Client {
	client, err := api.NewClient(api.Config{
		Address: "http://116.56.140.23:30213",
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}
	return client
}

func QueryNetUsageByNode(nodeName string) int64 {
	//首先根据client获取v1的api
	v1api := v1.NewAPI(PrometheusClient)
	//创建一个上下文，用于取消或超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	querystr := fmt.Sprintf("irate(node_network_transmit_bytes_total{instance=~\"%v.*\"}[60m]) > 0", NodeIP[nodeName])
	DPrinter("执行PromQL:" + querystr + "\n")
	result, warnings, err := v1api.Query(ctx, querystr, time.Now(), v1.WithTimeout(5*time.Second))
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	DPrinter("查询结果: %v\n", result)
	return 0
}
