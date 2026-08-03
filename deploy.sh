#!/bin/bash
# 部署/更新脚本：在服务器上、本项目根目录下执行
#   chmod +x deploy.sh
#   ./deploy.sh
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f .env ]; then
  echo "首次部署，生成随机数据库密码到 .env ..."
  ROOT_PW=$(openssl rand -hex 16)
  USER_PW=$(openssl rand -hex 16)
  cat > .env <<EOF
MYSQL_ROOT_PASSWORD=${ROOT_PW}
MYSQL_PASSWORD=${USER_PW}
EOF
  echo ".env 已生成，请妥善保管（里面是数据库密码，不要提交到公开仓库）"
fi

echo "构建并启动服务（首次会比较慢，需要下载基础镜像和 Go 依赖）..."
docker compose up -d --build

echo ""
echo "部署完成。"
echo "浏览器访问: http://101.42.45.60:8080"
echo ""
echo "如果访问不了，大概率是端口没放行，检查："
echo "  1) 云服务商控制台的安全组是否放行 8080 端口（阿里云/腾讯云等常见坑点，防火墙这一层容易漏配）"
echo "  2) 服务器本机防火墙: ufw allow 8080 (如果启用了 ufw)"
echo ""
echo "查看后端日志: docker compose logs -f backend"
echo "查看数据库日志: docker compose logs -f mysql"
echo "停止服务: docker compose down"
