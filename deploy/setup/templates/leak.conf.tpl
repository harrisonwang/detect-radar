# DNS 泄露信标站点：<token>.${LEAK_ZONE} 的 HTTP(S) 落点，只回像素/204。
# 真正的检测信号在 CoreDNS dnstap 一侧，这里保持最小化并关闭访问日志。
server {
    listen 80;
    listen [::]:80;
    server_name *.${LEAK_ZONE};
    access_log off;

    location = /pixel.gif {
        empty_gif;
        add_header Cache-Control "no-store" always;
    }
    location / { return 204; }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name *.${LEAK_ZONE};

    ssl_certificate     /etc/nginx/certs/${LEAK_ZONE}/fullchain.cer;
    ssl_certificate_key /etc/nginx/certs/${LEAK_ZONE}/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    access_log off;

    location = /pixel.gif {
        empty_gif;
        add_header Cache-Control "no-store" always;
    }
    location / { return 204; }
}
