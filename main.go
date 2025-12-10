package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	alidns20150109 "github.com/alibabacloud-go/alidns-20150109/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

func newClient(accessKeyId, accessKeySecret string) (*alidns20150109.Client, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(accessKeyId),
		AccessKeySecret: tea.String(accessKeySecret),
	}
	cfg.Endpoint = tea.String("alidns.cn-hangzhou.aliyuncs.com")
	return alidns20150109.NewClient(cfg)
}

func findRecordId(client *alidns20150109.Client, domain, rr, typ string) (string, error) {
	req := &alidns20150109.DescribeDomainRecordsRequest{
		DomainName:  tea.String(domain),
		RRKeyWord:   tea.String(rr),
		TypeKeyWord: tea.String(typ),
	}
	runtime := &util.RuntimeOptions{}
	resp, err := client.DescribeDomainRecordsWithOptions(req, runtime)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Body == nil || resp.Body.DomainRecords == nil || resp.Body.DomainRecords.Record == nil {
		return "", nil
	}
	for _, r := range resp.Body.DomainRecords.Record {
		// match exact rr and type
		if strings.EqualFold(tea.StringValue(r.RR), rr) && strings.EqualFold(tea.StringValue(r.Type), typ) {
			return tea.StringValue(r.RecordId), nil
		}
	}
	return "", nil
}

func updateRecord(client *alidns20150109.Client, recordId, rr, typ, value string, ttl int64, priority int64, line string) error {
	req := &alidns20150109.UpdateDomainRecordRequest{
		RecordId: tea.String(recordId),
		RR:       tea.String(rr),
		Type:     tea.String(typ),
		Value:    tea.String(value),
	}
	if ttl > 0 {
		req.TTL = tea.Int64(ttl)
	}
	if priority > 0 {
		req.Priority = tea.Int64(priority)
	}
	if line != "" {
		req.Line = tea.String(line)
	}
	runtime := &util.RuntimeOptions{}
	_, err := client.UpdateDomainRecordWithOptions(req, runtime)
	return err
}

func addRecord(client *alidns20150109.Client, domain, rr, typ, value string, ttl int64, priority int64, line string) error {
	req := &alidns20150109.AddDomainRecordRequest{
		DomainName: tea.String(domain),
		RR:         tea.String(rr),
		Type:       tea.String(typ),
		Value:      tea.String(value),
	}
	if ttl > 0 {
		req.TTL = tea.Int64(ttl)
	}
	if priority > 0 {
		req.Priority = tea.Int64(priority)
	}
	if line != "" {
		req.Line = tea.String(line)
	}
	runtime := &util.RuntimeOptions{}
	_, err := client.AddDomainRecordWithOptions(req, runtime)
	return err
}

func main() {
	var (
		domain          string
		rr              string
		typ             string
		value           string
		ttl             int
		priority        int
		line            string
		createIfMissing bool
	)

	flag.StringVar(&domain, "domain", "", "域名，例如 example.com")
	flag.StringVar(&rr, "rr", "@", "主机记录，例如 @ 或 www")
	flag.StringVar(&typ, "type", "A", "记录类型，例如 A/CNAME/TXT/MX 等")
	flag.StringVar(&value, "value", "", "记录值，例如 1.2.3.4 或 目标域名")
	flag.IntVar(&ttl, "ttl", 600, "TTL，单位秒")
	flag.IntVar(&priority, "priority", 0, "MX 记录优先级，仅对 MX 有效")
	flag.StringVar(&line, "line", "default", "解析线路，例如 default")
	flag.BoolVar(&createIfMissing, "create-if-missing", true, "当记录不存在时自动创建")
	flag.Parse()

	if domain == "" || rr == "" || typ == "" || value == "" {
		fmt.Fprintln(os.Stderr, "参数错误：必须提供 --domain、--rr、--type、--value")
		os.Exit(2)
	}

	ak := os.Getenv("ALICLOUD_ACCESS_KEY_ID")
	sk := os.Getenv("ALICLOUD_ACCESS_KEY_SECRET")
	if ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "缺少凭证：请设置环境变量 ALICLOUD_ACCESS_KEY_ID 和 ALICLOUD_ACCESS_KEY_SECRET")
		os.Exit(2)
	}

	client, err := newClient(ak, sk)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化客户端失败：", err)
		os.Exit(1)
	}

	recordId, err := findRecordId(client, domain, rr, typ)
	if err != nil {
		fmt.Fprintln(os.Stderr, "查询记录失败：", err)
		os.Exit(1)
	}

	if recordId == "" {
		if !createIfMissing {
			fmt.Fprintln(os.Stderr, "未找到匹配记录，且未启用自动创建")
			os.Exit(3)
		}
		if err := addRecord(client, domain, rr, typ, value, int64(ttl), int64(priority), line); err != nil {
			fmt.Fprintln(os.Stderr, "创建记录失败：", err)
			os.Exit(1)
		}
		fmt.Println("已创建记录", rr+"."+domain, typ, value)
		return
	}

	if err := updateRecord(client, recordId, rr, typ, value, int64(ttl), int64(priority), line); err != nil {
		fmt.Fprintln(os.Stderr, "更新记录失败：", err)
		os.Exit(1)
	}
	fmt.Println("已更新记录", rr+"."+domain, typ, value)
}
