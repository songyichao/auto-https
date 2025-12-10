auto-https 使用指南（Anolis OS 8.5）

一、这是什么
- auto-https 帮你自动更新网站证书并切换解析；也可以只上传证书到七牛并为域名启用新证书。
- 不需要懂技术，跟着步骤执行即可。

二、适用系统
- 适配 Anolis OS 8.5（以及 RHEL/CentOS 系列）。

三、下载与解压
- 把压缩包拷贝到服务器：`auto-https-anolis8.5-amd64.tar.gz` 或 `auto-https-anolis8.5-arm64.tar.gz`
- 解压：`tar -xzf auto-https-anolis8.5-amd64.tar.gz`
- 进入目录：`cd auto-https`

四、目录说明
- `bin/`
  - `rotate-cert`：证书自动轮换与七牛证书替换
  - `alidns-update`：阿里云解析记录查询与修改
- `state/`
  - `state.json`：记录上次替换时间，避免太频繁操作（初始值为 0）

五、准备工作（只需一次）
- 安装 Nginx 与 Certbot（若已安装可跳过）
- 设置阿里云凭证（用于解析切换）：
  - `export ALICLOUD_ACCESS_KEY_ID="你的AK"`
  - `export ALICLOUD_ACCESS_KEY_SECRET="你的SK"`
- 设置七牛凭证（用于证书上传与绑定）：
  - `export QINIU_ACCESS_KEY="你的AK"`
  - `export QINIU_SECRET_KEY="你的SK"`
- 持久化到当前用户：把以上 `export` 写入 `~/.bashrc` 或 `~/.zshrc`，然后执行 `source ~/.bashrc` 或 `source ~/.zshrc`
- 查找 Nginx 路径（如与默认不一致）：
  - `which nginx` 或 `nginx -V`（得到实际路径，例如 `/usr/sbin/nginx`）

六、推荐：一步一步交互运行
- 进入目录：`cd auto-https`
- 启动交互模式：`./bin/rotate-cert --interactive`
- 根据提示输入：
  - 模式：完整轮换 或 仅上传到七牛
  - `domain` 和主机记录 `rr-a`、`rr-b`（主机记录可在阿里云解析控制台查看）
  - `cert-domain`（可选：七牛上的域名，如 `cdn.example.com`，或证书目录名）
  - 是否忽略 89 天检查（默认否）
- 程序最后会显示“选择摘要”，并提示“确认执行? [y/N]”。输入 `y` 开始执行。

七、常用命令（非交互）
- 完整轮换（含解析切换、证书续期、Nginx 重载）：
  - `./bin/rotate-cert --domain example.com --rr-a a --rr-b b --state ./state/state.json`
- 仅上传到七牛并替换域名证书：
  - `./bin/rotate-cert --qiniu-only --cert-domain cdn.example.com`
- 指定鉴权模式（一般不需要）：
  - `./bin/rotate-cert --qiniu-only --cert-domain cdn.example.com --qiniu-token v1`

八、参数说明与取值方式（rotate-cert）
- `--domain`：基础域名，例如 `example.com`
  - 取值方式：你的网站主域名
- `--rr-a`：需要暂停的主机记录，例如 `a`
  - 取值方式：阿里云解析控制台的“主机记录”字段
- `--rr-b`：需要启用的主机记录，例如 `b`
  - 取值方式：阿里云解析控制台的“主机记录”字段
- `--type`：记录类型（可选），例如 `A`
  - 取值方式：阿里云解析控制台的“类型”（如 `A`、`CNAME`）。留空表示不限制类型
- `--value-a`：a 记录值（可选，用于匹配过滤）
  - 取值方式：阿里云解析控制台的“记录值”（如 IP 地址）。不会修改记录值，仅用于匹配
- `--value-b`：b 记录值（可选，用于匹配过滤）
  - 取值方式同上
- `--state`：状态文件路径，默认 `./state/state.json`
  - 作用：避免 89 天内重复执行。若要忽略检查，使用 `--force`
- `--force`：忽略 89 天检查强制执行，默认否
- `--certbot-live`：Certbot 证书目录，默认 `/etc/letsencrypt/live`
  - 取值方式：通常为默认值；如果部署自定义位置，改为对应路径
- `--cert-domain`：证书域名（可选）
  - 取值方式：七牛的域名（如 `cdn.example.com`），或 Certbot 目录名。如果不填，默认使用 `rr-a.domain`
- `--qiniu-ak`、`--qiniu-sk`：七牛 AK/SK（可用环境变量 `QINIU_ACCESS_KEY`、`QINIU_SECRET_KEY`）
  - 取值方式：登录七牛开发者平台 → 密钥管理
- `--nginx`：Nginx 可执行文件路径，默认 `/usr/local/nginx/sbin/nginx`
  - 取值方式：`which nginx` 或 `nginx -V`
- `--qiniu-only`：仅上传证书到七牛并为域名替换证书；跳过解析切换、续期与状态记录
- `--qiniu-token`：七牛鉴权模式 `auto|v1|v2`（默认 `auto`）
  - 说明：一般保持默认；如遇鉴权异常可手动指定
- `--interactive`：交互式模式（推荐初次使用）

九、运行示例（一步到位）
- 完整轮换：
  - `export ALICLOUD_ACCESS_KEY_ID="你的AK"`
  - `export ALICLOUD_ACCESS_KEY_SECRET="你的SK"`
  - `export QINIU_ACCESS_KEY="你的AK"`
  - `export QINIU_SECRET_KEY="你的SK"`
  - `cd auto-https && ./bin/rotate-cert --domain example.com --rr-a a --rr-b b --state ./state/state.json`
- 仅上传到七牛：
  - `export QINIU_ACCESS_KEY="你的AK"`
  - `export QINIU_SECRET_KEY="你的SK"`
  - `cd auto-https && ./bin/rotate-cert --qiniu-only --cert-domain cdn.example.com`

十、提示与说明
- 记录值过滤：设置 `--value-a/--value-b` 会严格匹配对应记录，不会改动记录值
- 证书自动选择：会选取最新的 `privkeyN.pem` 与 `fullchainN.pem` 配对文件
- 状态文件：默认 `./state/state.json`，可自定义路径

十一、常见问题
- 环境变量未生效：执行 `echo $ALICLOUD_ACCESS_KEY_ID` 等检查；如无值，请重新 `export` 或 `source ~/.bashrc`
- 七牛 BadToken：确保 AK/SK 与 `cert-domain` 属于同一七牛账户，且该域名在融合 CDN 接入并允许 HTTPS 配置
- 找不到记录：确认阿里云解析的主机记录、类型与记录值是否与命令一致
- Nginx 路径不对：使用 `which nginx` 或 `nginx -V` 查找实际路径，并通过 `--nginx` 指定

十二、安全建议
- 不要把密钥写入代码或上传到仓库；使用环境变量保存。

十三、日志与反馈
- 程序会在终端打印执行过程；如遇报错，请根据提示处理或联系维护者。
