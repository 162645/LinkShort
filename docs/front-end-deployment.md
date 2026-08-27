# 前端部署详细教程

本项目前端采用了轻量级的 Vanilla 技术栈（HTML/CSS/JS），支持多种部署方式。

## 方案一：后端集成部署（推荐，最简单）
由于 `rest-api-svc` 已经集成了静态文件服务，你可以直接通过后端访问前端。

1.  **启动后端服务**:
    ```bash
    make run-rest
    # 或者使用 Docker
    docker-compose up -d rest-api-service
    ```
2.  **访问地址**: 
    打开浏览器访问 `http://localhost:8080/` 即可看到现代化 UI。

---

## 方案二：Nginx 独立部署（高性能）
适合生产环境，将静态资源交由 Nginx 处理。

1.  **准备文件**: 将 `frontend` 目录下的所有文件上传至服务器目录（如 `/var/www/url-shortener`）。
2.  **配置 Nginx**:
    ```nginx
    server {
        listen 80;
        server_name your-domain.com;

        root /var/www/url-shortener;
        index index.html;

        # 静态资源缓存控制
        location ~* \.(css|js|png|jpg|svg)$ {
            expires 7d;
            add_header Cache-Control "public";
        }

        # 接口转发 (关键: 将前端 API 请求转发到 Go 后端)
        location /api/ {
            proxy_pass http://127.0.0.1:8080;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }

        # 短链接跳转转发
        location ~ ^/[a-zA-Z0-9]+$ {
            proxy_pass http://127.0.0.1:8080;
        }

        # 默认路由
        location / {
            try_files $uri $uri/ /index.html;
        }
    }
    ```

---

## 方案三：Docker 容器化部署
如果你希望使用独立的容器运行前端。

1.  **创建 Dockerfile**:
    ```dockerfile
    FROM nginx:stable-alpine
    COPY ./frontend /usr/share/nginx/html
    # 可选: 复制自定义 nginx 配置覆盖默认配置以支持 API 转发
    # COPY ./nginx.conf /etc/nginx/conf.d/default.conf
    EXPOSE 80
    CMD ["nginx", "-g", "daemon off;"]
    ```
2.  **构建并运行**:
    ```bash
    docker build -t url-shortener-web .
    docker run -d -p 80:80 url-shortener-web
    ```

## ⚠️ 常见问题说明
- **跨域问题 (CORS)**: 如果前端通过 `http://localhost:3000` 访问 `http://localhost:8080` 的 API，我已经同步更新了 `rest-api-svc` 以支持所有源的跨域请求。
- **配置 API 地址**: 目前 `app.js` 使用 `window.location.origin` 作为基地址，这意味着它默认尝试访问同端口的 API。如果你的后端运行在不同端口，请在 `app.js` 第一行修改 `API_BASE`。
