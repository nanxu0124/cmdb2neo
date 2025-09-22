package unit

import (
	"testing"

	"cmdb2neo/internal/domain"
)

func TestLabelPattern(t *testing.T) {
	pattern := domain.LabelPattern([]string{"Compute", "VirtualMachine"})
	if pattern != ":Compute:VirtualMachine" {
		t.Fatalf("unexpected pattern %s", pattern)
	}
}

type AppObj struct {
	Id         int    `json:"id"`
	FullCNName string `json:"full_cn_name"`
}
type DataContent struct {
	Id               int    `json:"id"`
	Idc              string `json:"idc"`
	NetworkPartition string `json:"network_partition"`
	ServerType       int    `json:"server_type"`
	Ip               string `json:"ip"`
	HostName         string `json:"host_name"`
	HostIp           string `json:"host_ip"`
	AppObj           AppObj `json:"app_obj"`
}

type ResponseData struct {
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
	Total int           `json:"total"`
	Data  []DataContent `json:"data"`
}

type Request struct {
	Code int          `json:"code"`
	Data ResponseData `json:"data"`
	Msg  string       `json:"msg"`
}
