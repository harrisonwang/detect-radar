server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    # 证书走 DNS-01，不需要 HTTP-01 webroot，80 只做跳转
    location / {
        return 301 https://${DOMAIN}$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ${DOMAIN};

    ssl_certificate     /etc/nginx/certs/${DOMAIN}/fullchain.cer;
    ssl_certificate_key /etc/nginx/certs/${DOMAIN}/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384';

    root /usr/share/nginx/site;
    index index.html;

    client_max_body_size 1m;

    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Content-Type-Options "nosniff" always;

    location = /health {
        proxy_pass http://127.0.0.1:8080/api/v1/health;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Connection "";
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080/api/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Connection "";
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;

        # 出口 IP↔本机的 TCP 往返时延（µs）。真值只有持有客户端连接的 nginx 看得见；
        # proxy_set_header 会覆写客户端自带的同名头，杜绝伪造。见 docs/rtt-phase1.md。
        proxy_set_header X-TCP-RTT    $tcpinfo_rtt;
        proxy_set_header X-TCP-RTTVAR $tcpinfo_rttvar;

        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    location /_astro/ {
        try_files $uri =404;
        add_header Cache-Control "public, max-age=31536000, immutable";
        access_log off;
    }

    location ~* \.(jpg|jpeg|png|gif|ico|css|js|svg|webp|woff2?)$ {
        try_files $uri =404;
        expires 30d;
        add_header Cache-Control "public, immutable";
        access_log off;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }

    location ~ /\. {
        deny all;
        access_log off;
        log_not_found off;
    }
}
