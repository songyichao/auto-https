package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	alidns20150109 "github.com/alibabacloud-go/alidns-20150109/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
)

type state struct {
	LastReplaceUnix int64 `json:"last_replace_unix"`
}

var qiniuTokenMode string

func newClient(accessKeyId, accessKeySecret string) (*alidns20150109.Client, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(accessKeyId),
		AccessKeySecret: tea.String(accessKeySecret),
	}
	cfg.Endpoint = tea.String("alidns.cn-hangzhou.aliyuncs.com")
	return alidns20150109.NewClient(cfg)
}

func listRecords(client *alidns20150109.Client, domain string) ([]*alidns20150109.DescribeDomainRecordsResponseBodyDomainRecordsRecord, error) {
	req := &alidns20150109.DescribeDomainRecordsRequest{
		DomainName: tea.String(domain),
	}
	runtime := &util.RuntimeOptions{}
	resp, err := client.DescribeDomainRecordsWithOptions(req, runtime)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil || resp.Body.DomainRecords == nil {
		return nil, nil
	}
	return resp.Body.DomainRecords.Record, nil
}

func findRecordIdExact(records []*alidns20150109.DescribeDomainRecordsResponseBodyDomainRecordsRecord, rr, typ, value string) string {
	for _, r := range records {
		if !strings.EqualFold(tea.StringValue(r.RR), rr) {
			continue
		}
		if typ != "" && !strings.EqualFold(tea.StringValue(r.Type), typ) {
			continue
		}
		if value != "" && !strings.EqualFold(tea.StringValue(r.Value), value) {
			continue
		}
		return tea.StringValue(r.RecordId)
	}
	return ""
}

func setRecordStatus(client *alidns20150109.Client, recordId string, enable bool) error {
	status := "DISABLE"
	if enable {
		status = "ENABLE"
	}
	req := &alidns20150109.SetDomainRecordStatusRequest{
		RecordId: tea.String(recordId),
		Status:   tea.String(status),
	}
	runtime := &util.RuntimeOptions{}
	_, err := client.SetDomainRecordStatusWithOptions(req, runtime)
	return err
}

// removed value update; matching uses value when provided

func readState(path string) (*state, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return &state{LastReplaceUnix: 0}, nil
	}
	s := &state{}
	txt := string(b)
	i := strings.Index(txt, "last_replace_unix")
	if i >= 0 {
		j := strings.LastIndex(txt, ":")
		if j >= 0 {
			val := strings.Trim(txt[j+1:], " \n\t\r}")
			if u, err := parseInt64(val); err == nil {
				s.LastReplaceUnix = u
			}
		}
	}
	return s, nil
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			continue
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

func writeState(path string, ts int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("{\n  \"last_replace_unix\": %d\n}\n", ts)
	return os.WriteFile(path, []byte(content), 0o644)
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Println(string(out))
	}
	return err
}

func qboxAccessToken(ak, sk, method, rawURL, contentType string, body []byte) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	signingStr := method + " " + u.Path
	if u.RawQuery != "" {
		signingStr += "?" + u.RawQuery
	}
	signingStr += "\nHost: " + u.Host + "\n"
	if contentType != "" {
		signingStr += "Content-Type: " + contentType + "\n"
	}
	signingStr += "\n"
	if len(body) > 0 && contentType != "" && contentType != "application/octet-stream" {
		signingStr += string(body)
	}
	mac := hmac.New(sha1.New, []byte(sk))
	mac.Write([]byte(signingStr))
	sign := mac.Sum(nil)
	encodedSign := base64.URLEncoding.EncodeToString(sign)
	return "QBox " + ak + ":" + encodedSign, nil
}

func qiniuTokenCandidates(ak, sk, method, rawURL, contentType string, body []byte) []string {
	tokens := []string{}
	req, _ := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	mac := qbox.NewMac(ak, sk)
	if tk, err := mac.SignRequest(req); err == nil {
		tokens = append(tokens, "QBox "+tk)
	}
	if tk, err := mac.SignRequestV2(req); err == nil {
		tokens = append(tokens, tk)
	}
	return tokens
}

func qiniuUploadCert(ak, sk, certDomain, name, pri, ca string) (string, error) {
	urlStr := "https://api.qiniu.com/sslcert"
	body := fmt.Sprintf("{\"name\":\"%s\",\"common_name\":\"%s\",\"pri\":%q,\"ca\":%q}", name, certDomain, pri, ca)
	tokens := qiniuTokenCandidates(ak, sk, http.MethodPost, urlStr, "application/json", []byte(body))
	if qiniuTokenMode == "v1" {
		if len(tokens) > 1 {
			tokens = tokens[:1]
		}
	} else if qiniuTokenMode == "v2" {
		if len(tokens) > 1 {
			tokens = tokens[1:]
		}
	}
	var s string
	for _, tk := range tokens {
		req, _ := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(body))
		req.Header.Set("Authorization", tk)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			s = err.Error()
			continue
		}
		defer resp.Body.Close()
		rb, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			s = string(rb)
			continue
		}
		s = string(rb)
		break
	}
	if s == "" || strings.Contains(s, "BadToken") || strings.HasPrefix(s, "error") {
		return "", fmt.Errorf("qiniu upload failed: %s", s)
	}
	{
		s2 := s
		idx := strings.Index(s2, "\"certID\"")
		if idx < 0 {
			return "", fmt.Errorf("no certID in response: %s", s2)
		}
		colon := strings.Index(s2[idx:], ":")
		if colon < 0 {
			return "", fmt.Errorf("bad response: %s", s2)
		}
		rest := s2[idx+colon+1:]
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "\"") {
			rest = rest[1:]
			end := strings.Index(rest, "\"")
			if end > 0 {
				return rest[:end], nil
			}
		}
		end := strings.IndexAny(rest, ",}\n")
		if end > 0 {
			return strings.TrimSpace(rest[:end]), nil
		}
		return strings.TrimSpace(rest), nil
	}
}

func qiniuBindDomainCert(ak, sk, cdnDomain, certID string) error {
	urlStr := fmt.Sprintf("https://api.qiniu.com/domain/%s/httpsconf", cdnDomain)
	body := fmt.Sprintf("{\"certId\":\"%s\",\"forceHttps\":false,\"http2Enable\":true}", certID)
	tokens := qiniuTokenCandidates(ak, sk, http.MethodPut, urlStr, "application/json", []byte(body))
	if qiniuTokenMode == "v1" {
		if len(tokens) > 1 {
			tokens = tokens[:1]
		}
	} else if qiniuTokenMode == "v2" {
		if len(tokens) > 1 {
			tokens = tokens[1:]
		}
	}
	var last string
	for _, tk := range tokens {
		req, _ := http.NewRequest(http.MethodPut, urlStr, strings.NewReader(body))
		req.Header.Set("Authorization", tk)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			last = err.Error()
			continue
		}
		defer resp.Body.Close()
		rb, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			last = string(rb)
			continue
		}
		return nil
	}
	return fmt.Errorf("qiniu bind failed: %s", last)
}

func findLatestCertPair(liveDir, preferredDomain string) (domain string, privPath string, fullchainPath string, err error) {
	if preferredDomain != "" {
		d := filepath.Join(liveDir, preferredDomain)
		if _, err := os.Stat(d); err == nil {
			pv, fc, err := pickNumberedCertFiles(d)
			if err == nil {
				return preferredDomain, pv, fc, nil
			}
		}
	}
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		return "", "", "", err
	}
	type item struct {
		name string
		t    time.Time
	}
	var items []item
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(liveDir, e.Name())
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		items = append(items, item{name: e.Name(), t: fi.ModTime()})
	}
	if len(items) == 0 {
		return "", "", "", fmt.Errorf("no certbot live entries under %s", liveDir)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].t.After(items[j].t) })
	d := filepath.Join(liveDir, items[0].name)
	pv, fc, err := pickNumberedCertFiles(d)
	if err != nil {
		return "", "", "", err
	}
	return items[0].name, pv, fc, nil
}

func pickNumberedCertFiles(dir string) (privPath string, fullchainPath string, err error) {
	rePriv := regexp.MustCompile(`^privkey(\d*)\.pem$`)
	reFull := regexp.MustCompile(`^fullchain(\d*)\.pem$`)
	privs := make(map[int]string)
	fulls := make(map[int]string)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", "", err
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if m := rePriv.FindStringSubmatch(name); m != nil {
			n := 0
			if m[1] != "" {
				for i := 0; i < len(m[1]); i++ {
					n = n*10 + int(m[1][i]-'0')
				}
			}
			privs[n] = filepath.Join(dir, name)
		} else if m := reFull.FindStringSubmatch(name); m != nil {
			n := 0
			if m[1] != "" {
				for i := 0; i < len(m[1]); i++ {
					n = n*10 + int(m[1][i]-'0')
				}
			}
			fulls[n] = filepath.Join(dir, name)
		}
	}
	if len(privs) == 0 || len(fulls) == 0 {
		return "", "", fmt.Errorf("no numbered cert files under %s", dir)
	}
	best := -1
	for n := range privs {
		if _, ok := fulls[n]; ok {
			if n > best {
				best = n
			}
		}
	}
	if best >= 0 {
		return privs[best], fulls[best], nil
	}
	largestPriv := -1
	largestFull := -1
	for n := range privs {
		if n > largestPriv {
			largestPriv = n
		}
	}
	for n := range fulls {
		if n > largestFull {
			largestFull = n
		}
	}
	if largestPriv >= 0 && largestFull >= 0 {
		return privs[largestPriv], fulls[largestFull], nil
	}
	return "", "", fmt.Errorf("unable to select cert files under %s", dir)
}

func main() {
	var (
		domain         string
		rrA            string
		rrB            string
		typ            string
		valueA         string
		valueB         string
		statePath      string
		force          bool
		certbotLiveDir string
		certDomain     string
		qiniuAK        string
		qiniuSK        string
		nginxBin       string
		qiniuOnly      bool
		qiniuTokenMode string
		interactive    bool
	)

	flag.StringVar(&domain, "domain", "", "基础域名，例如 example.com")
	flag.StringVar(&rrA, "rr-a", "a", "需要暂停的主机记录，例如 a")
	flag.StringVar(&rrB, "rr-b", "b", "需要启用的主机记录，例如 b")
	flag.StringVar(&typ, "type", "", "记录类型（可选），例如 A")
	flag.StringVar(&valueA, "value-a", "", "a记录值")
	flag.StringVar(&valueB, "value-b", "", "b记录值")
	flag.StringVar(&statePath, "state", "./state/state.json", "替换时间状态文件路径")
	flag.BoolVar(&force, "force", false, "忽略89天检查强制执行")
	flag.StringVar(&certbotLiveDir, "certbot-live", "/etc/letsencrypt/live", "certbot证书目录")
	flag.StringVar(&certDomain, "cert-domain", "", "证书域名（默认为最新目录或与CDN域名一致）")
	flag.StringVar(&qiniuAK, "qiniu-ak", os.Getenv("QINIU_ACCESS_KEY"), "七牛AK（可用环境变量QINIU_ACCESS_KEY）")
	flag.StringVar(&qiniuSK, "qiniu-sk", os.Getenv("QINIU_SECRET_KEY"), "七牛SK（可用环境变量QINIU_SECRET_KEY）")
	flag.StringVar(&nginxBin, "nginx", "/usr/local/nginx/sbin/nginx", "nginx可执行文件路径")
	flag.BoolVar(&qiniuOnly, "qiniu-only", false, "仅上传证书到七牛并为域名替换证书")
	flag.StringVar(&qiniuTokenMode, "qiniu-token", "auto", "七牛鉴权模式：auto|v1|v2")
	flag.BoolVar(&interactive, "interactive", false, "交互式模式")
	flag.Parse()

	if interactive {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("选择模式 [1=完整轮换, 2=仅七牛证书上传替换] (默认1): ")
		m, _ := reader.ReadString('\n')
		m = strings.TrimSpace(m)
		if m == "2" {
			qiniuOnly = true
		}
		fmt.Print("输入基础域名 (例如 example.com)，仅七牛模式可留空: ")
		d, _ := reader.ReadString('\n')
		d = strings.TrimSpace(d)
		if d != "" {
			domain = d
		}
		if !qiniuOnly {
			fmt.Print("输入需要暂停的主机记录 rr-a (默认 a): ")
			a, _ := reader.ReadString('\n')
			a = strings.TrimSpace(a)
			if a != "" {
				rrA = a
			} else if rrA == "" {
				rrA = "a"
			}
			fmt.Print("输入需要启用的主机记录 rr-b (默认 b): ")
			b, _ := reader.ReadString('\n')
			b = strings.TrimSpace(b)
			if b != "" {
				rrB = b
			} else if rrB == "" {
				rrB = "b"
			}
			fmt.Print("输入记录类型 type (可选，示例 A，留空表示不限制): ")
			t, _ := reader.ReadString('\n')
			t = strings.TrimSpace(t)
			if t != "" {
				typ = t
			}
			fmt.Print("a记录值过滤 (可选): ")
			va, _ := reader.ReadString('\n')
			va = strings.TrimSpace(va)
			if va != "" {
				valueA = va
			}
			fmt.Print("b记录值过滤 (可选): ")
			vb, _ := reader.ReadString('\n')
			vb = strings.TrimSpace(vb)
			if vb != "" {
				valueB = vb
			}
		}
		fmt.Print("证书域名 cert-domain (可选，默认使用最新目录或 rr-a.domain): ")
		cd, _ := reader.ReadString('\n')
		cd = strings.TrimSpace(cd)
		if cd != "" {
			certDomain = cd
		}
		if qiniuAK == "" {
			fmt.Print("七牛 AccessKey (回车沿用环境变量): ")
			akIn, _ := reader.ReadString('\n')
			akIn = strings.TrimSpace(akIn)
			if akIn != "" {
				qiniuAK = akIn
			}
		}
		if qiniuSK == "" {
			fmt.Print("七牛 SecretKey (回车沿用环境变量): ")
			skIn, _ := reader.ReadString('\n')
			skIn = strings.TrimSpace(skIn)
			if skIn != "" {
				qiniuSK = skIn
			}
		}
		fmt.Print("七牛鉴权模式 [auto|v1|v2] (默认 auto): ")
		tm, _ := reader.ReadString('\n')
		tm = strings.TrimSpace(tm)
		if tm == "v1" || tm == "v2" {
			qiniuTokenMode = tm
		}
		fmt.Print("是否忽略89天检查 [y/N]: ")
		fm, _ := reader.ReadString('\n')
		fm = strings.TrimSpace(strings.ToLower(fm))
		if fm == "y" || fm == "yes" {
			force = true
		}
		fmt.Printf("模式:%s 域名:%s rr-a:%s rr-b:%s type:%s cert-domain:%s qiniu-only:%v\n",
			map[bool]string{true: "仅七牛", false: "完整轮换"}[qiniuOnly], domain, rrA, rrB, typ, certDomain, qiniuOnly)
		fmt.Print("确认执行? [y/N]: ")
		ok, _ := reader.ReadString('\n')
		ok = strings.TrimSpace(strings.ToLower(ok))
		if ok != "y" && ok != "yes" {
			fmt.Println("已取消")
			return
		}
	}
	if domain == "" && certDomain == "" {
		fmt.Fprintln(os.Stderr, "必须提供 --domain 或 --cert-domain")
		os.Exit(2)
	}

	ak := os.Getenv("ALICLOUD_ACCESS_KEY_ID")
	sk := os.Getenv("ALICLOUD_ACCESS_KEY_SECRET")
	if !qiniuOnly {
		if ak == "" || sk == "" {
			fmt.Fprintln(os.Stderr, "缺少阿里云凭证：请设置 ALICLOUD_ACCESS_KEY_ID 和 ALICLOUD_ACCESS_KEY_SECRET")
			os.Exit(2)
		}
	}

	if !qiniuOnly {
		st, _ := readState(statePath)
		now := time.Now().Unix()
		if !force && st.LastReplaceUnix != 0 && now-st.LastReplaceUnix < 89*24*3600 {
			fmt.Printf("距离上次替换未超过89天（%d天），本次跳过。\n", (now-st.LastReplaceUnix)/86400)
			return
		}
	}

	var client *alidns20150109.Client
	var err error
	if !qiniuOnly {
		client, err = newClient(ak, sk)
		if err != nil {
			fmt.Fprintln(os.Stderr, "初始化阿里云DNS客户端失败：", err)
			os.Exit(1)
		}
	}

	var aID, bID string
	if !qiniuOnly {
		records, err := listRecords(client, domain)
		if err != nil {
			fmt.Fprintln(os.Stderr, "查询云解析记录失败：", err)
			os.Exit(1)
		}
		fmt.Printf("云解析记录总数：%d\n", len(records))

		aID = findRecordIdExact(records, rrA, typ, valueA)
		bID = findRecordIdExact(records, rrB, typ, valueB)
		if aID == "" || bID == "" {
			fmt.Fprintln(os.Stderr, "未找到 a 或 b 主机记录，请检查 --rr-a/--rr-b 与记录类型/记录值")
			os.Exit(3)
		}
	}

	// do not update record values; values are only used for matching

	if !qiniuOnly {
		if err := setRecordStatus(client, aID, false); err != nil {
			fmt.Fprintln(os.Stderr, "暂停 a 主机记录失败：", err)
			os.Exit(1)
		}
		fmt.Println("已暂停记录", rrA+"."+domain)
		if err := setRecordStatus(client, bID, true); err != nil {
			fmt.Fprintln(os.Stderr, "启用 b 主机记录失败：", err)
			os.Exit(1)
		}
		fmt.Println("已启用记录", rrB+"."+domain)
	}

	if !qiniuOnly {
		if err := runCmd("certbot", "renew"); err != nil {
			fmt.Fprintln(os.Stderr, "执行 certbot renew 失败：", err)
		}
		if err := runCmd(nginxBin, "-s", "reload"); err != nil {
			fmt.Fprintln(os.Stderr, "重载 nginx 失败：", err)
		}
		if err := writeState(statePath, time.Now().Unix()); err != nil {
			fmt.Fprintln(os.Stderr, "写入替换时间失败：", err)
		}
	}

	cdnDomain := certDomain
	if cdnDomain == "" {
		cdnDomain = rrA + "." + domain
	}

	certDirDomain, privPath, fullchainPath, err := findLatestCertPair(certbotLiveDir, certDomain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "查找最新证书失败：", err)
	} else {
		priBytes, _ := os.ReadFile(privPath)
		caBytes, _ := os.ReadFile(fullchainPath)
		if qiniuAK == "" || qiniuSK == "" {
			fmt.Fprintln(os.Stderr, "缺少七牛AK/SK，跳过证书上传与替换")
		} else {
			certName := fmt.Sprintf("%s-letsencrypt-%s", certDirDomain, time.Now().Format("20060102"))
			certID, err := qiniuUploadCert(qiniuAK, qiniuSK, cdnDomain, certName, string(priBytes), string(caBytes))
			if err != nil {
				fmt.Fprintln(os.Stderr, "上传七牛证书失败：", err)
			} else {
				fmt.Println("七牛证书上传成功，certID:", certID)
				if err := qiniuBindDomainCert(qiniuAK, qiniuSK, cdnDomain, certID); err != nil {
					fmt.Fprintln(os.Stderr, "七牛域名证书替换失败：", err)
				} else {
					fmt.Println("已替换七牛 CDN 域名证书：", cdnDomain)
				}
			}
		}
	}

	if qiniuOnly {
		return
	}

	if err := setRecordStatus(client, bID, false); err != nil {
		fmt.Fprintln(os.Stderr, "暂停 b 主机记录失败：", err)
	} else {
		fmt.Println("已暂停记录", rrB+"."+domain)
	}
	if err := setRecordStatus(client, aID, true); err != nil {
		fmt.Fprintln(os.Stderr, "启用 a 主机记录失败：", err)
	} else {
		fmt.Println("已启用记录", rrA+"."+domain)
	}
}
