Caddy + xcaddy + fanwebcaddy 插件完整安装手册

环境：CentOS 7.9 x86_64 | 适用于 fanwebbaidu 泛域名 SSL 证书方案更新日期：2026-05-21

一、安装 Go

````bash
wget https://golang.google.cn/dl/go1.22.5.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
 
cat >> /etc/profile.d/go.sh << 'EOF'
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/root/go
export PATH=$PATH:/root/go/bin
EOF
 
source /etc/profile.d/go.sh
go version
```

期望输出：go version go1.22.5 linux/amd64

二、配置 Go 国内代理

````bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GONOSUMDB=*
```

期望输出：无报错输出

三、安装编译依赖

````bash
yum install -y git gcc
```

期望输出：Complete!

四、安装 xcaddy

````bash
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
xcaddy version
```

期望输出：v0.4.6 h1:/kbArNJZFPewjwlijr83WdssSuhSZ9XT2cDSWmonkjc=

五、编译带插件的 Caddy

⚠️  必须指定 v2.8.4，否则 xcaddy 拉取最新 Caddy 会导致 libdns API 不兼容报错。

````bash
mkdir -p /usr/local/caddy && cd /usr/local/caddy
 
xcaddy build v2.8.4 --with github.com/HYERIC/fanwebcaddy@v0.1.1
```

期望输出：[INFO] Build complete: ./caddy

验证编译结果：

````bash
./caddy version
./caddy list-modules | grep fanweb
```

期望输出：v2.8.4 ...dns.providers.fanweb

六、安装 Caddy 二进制

````bash
cp /usr/local/caddy/caddy /usr/bin/caddy
chmod +x /usr/bin/caddy
caddy version
```

期望输出：v2.8.4 h1:q3pe0wpBj1OcHFZ3n/1nl4V4bxBrYoSoab7rL9BMYNk=

七、创建运行用户和目录

````bash
groupadd --system caddy
useradd --system \
  --gid caddy \
  --create-home \
  --home-dir /var/lib/caddy \
  --shell /sbin/nologin \
  --comment "Caddy web server" \
  caddy
 
mkdir -p /etc/caddy /var/log/caddy
chown caddy:caddy /var/log/caddy /var/lib/caddy
```

期望输出：无报错输出

八、配置 systemd 服务

````bash
cat > /etc/systemd/system/caddy.service << 'EOF'
[Unit]
Description=Caddy
After=network.target network-online.target
Requires=network-online.target
 
[Service]
Type=notify
User=caddy
Group=caddy
ExecStart=/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
 
[Install]
WantedBy=multi-user.target
EOF
 
systemctl daemon-reload
systemctl enable caddy
```

期望输出：Created symlink ... caddy.service

九、写 Caddyfile

域名路由和 TLS 策略全部由 fanwebbaidu 通过 Admin API 动态推送，Caddyfile 只需全局配置。

````bash
cat > /etc/caddy/Caddyfile << 'EOF'
{
    email hczxheyi@gmail.com
    admin localhost:2019
}
EOF
 
caddy validate --config /etc/caddy/Caddyfile
```

期望输出：Valid configuration

十、开放防火墙端口

````bash
firewall-cmd --permanent --add-service=http
firewall-cmd --permanent --add-service=https
firewall-cmd --reload
firewall-cmd --list-services
```

期望输出：successsuccesssuccessdhcpv6-client http https ssh

十一、启动 Caddy

````bash
systemctl start caddy
systemctl status caddy
```

期望输出：Active: active (running)

验证 Admin API：

````bash
curl http://localhost:2019/config/
```

期望输出：{}

十二、配置 fanwebbaidu sys_config

执行以下 SQL（或在管理后台配置）：

````bash
UPDATE sys_config SET config_value = 'http://localhost:2019'
WHERE config_key = 'caddy.admin.url';
 
UPDATE sys_config SET config_value = 'localhost:9100'
WHERE config_key = 'caddy.proxy.target';
 
UPDATE sys_config SET config_value = 'http://zxdns地址:端口'
WHERE config_key = 'caddy.dns.endpoint';
 
UPDATE sys_config SET config_value = 'http://服务器IP:9100'
WHERE config_key = 'caddy.fanweb.endpoint';
```

十三、验证完整流程

````bash
# Caddy 进程正常
systemctl status caddy | grep Active
 
# 插件已加载
caddy list-modules | grep fanweb
 
# Admin API 可访问
curl -s http://localhost:2019/config/ | head -c 100
 
# 80/443 监听（域名推送后才出现）
ss -tlnp | grep caddy
```

日后插件更新流程

````bash
# 1. 本地打 tag 并推送（Windows）
git tag v0.1.x
git push origin main && git push origin v0.1.x
 
# 2. 服务器重新编译
source /etc/profile.d/go.sh
cd /usr/local/caddy
xcaddy build v2.8.4 --with github.com/HYERIC/fanwebcaddy@v0.1.x
 
# 3. 替换并重启
systemctl stop caddy
cp caddy /usr/bin/caddy
systemctl start caddy
caddy list-modules | grep fanweb
```

常见问题速查

现象

原因

解决方案

````bash
xcaddy: command not found
PATH 未加载
source /etc/profile.d/go.sh
libdns.Record has no field Type
xcaddy 拉取了最新 Caddy
xcaddy build v2.8.4 --with ...
wget go.dev SSL connection failed
go.dev 被墙
改用 golang.google.cn 镜像下载
unknown variable GONOSUMCHECK
变量名拼写错误
go env -w GONOSUMDB=*
Admin API 无响应
SELinux 拦截
setsebool -P httpd_can_network_connect 1
443 端口无法访问
firewalld 未开放
执行第十步防火墙命令
编译时网络超时
GitHub 连接慢
已配置 GOPROXY=goproxy.cn，重试即可
```

| 现象 | 原因 | 解决方案 |
| --- | --- | --- |
| xcaddy: command not found | PATH 未加载 | source /etc/profile.d/go.sh |
| libdns.Record has no field Type | xcaddy 拉取了最新 Caddy | xcaddy build v2.8.4 --with ... |
| wget go.dev SSL connection failed | go.dev 被墙 | 改用 golang.google.cn 镜像下载 |
| unknown variable GONOSUMCHECK | 变量名拼写错误 | go env -w GONOSUMDB=* |
| Admin API 无响应 | SELinux 拦截 | setsebool -P httpd_can_network_connect 1 |
| 443 端口无法访问 | firewalld 未开放 | 执行第十步防火墙命令 |
| 编译时网络超时 | GitHub 连接慢 | 已配置 GOPROXY=goproxy.cn，重试即可 |


