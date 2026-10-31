#!/bin/sh
set -eu

VERSION="${DOCKIUM_VERSION:-0.1.0}"
INSTALL_DIR="${DOCKIUM_DIR:-/opt/dockium}"
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT_TOKEN="${DOCKIUM_AGENT_TOKEN:-}"
AGENT_ENDPOINT="${DOCKIUM_AGENT_ENDPOINT:-localhost:3443}"
TRUST_PROXY="${DOCKIUM_TRUST_PROXY:-0}"
SECURE_COOKIES="${DOCKIUM_SECURE_COOKIES:-0}"
TIMEZONE="${TZ:-UTC}"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker не найден. Установите Docker 24+ и повторите." >&2
  exit 1
fi

if [ ! -f "$ROOT_DIR/src/go.mod" ]; then
  echo "Не найден исходный код Dockium: $ROOT_DIR/src/go.mod" >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  echo "Нужен плагин docker compose или docker-compose." >&2
  exit 1
fi

echo "==> Каталог установки: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"

if [ "${DOCKIUM_SKIP_BUILD:-0}" != "1" ]; then
  echo "==> Сборка образа панели"
  docker build --pull=false \
    -f "$ROOT_DIR/src/deploy/Dockerfile.web" \
    -t "dockium-web:$VERSION" \
    "$ROOT_DIR/src"
  echo "==> Сборка образа агента"
  docker build --pull=false \
    -f "$ROOT_DIR/src/deploy/Dockerfile.agent" \
    -t "dockium-agent:$VERSION" \
    "$ROOT_DIR/src"
fi

umask 077
if [ -n "$AGENT_TOKEN" ]; then
  printf 'DOCKIUM_AGENT_TOKEN=%s\n' "$AGENT_TOKEN" > "$INSTALL_DIR/.env"
else
  : > "$INSTALL_DIR/.env"
fi

{
  cat <<EOF
services:
  panel:
    image: dockium-web:${VERSION}
    container_name: dockium-web
    restart: unless-stopped
    ports:
      - "3434:3434"
      - "3443:3443"
    environment:
      TZ: "${TIMEZONE}"
      DOCKIUM_AGENT_ENDPOINT: "${AGENT_ENDPOINT}"
      DOCKIUM_TRUST_PROXY: "${TRUST_PROXY}"
      DOCKIUM_SECURE_COOKIES: "${SECURE_COOKIES}"
    volumes:
      - dockium-data:/data
EOF
  if [ -n "$AGENT_TOKEN" ]; then
    cat <<EOF
  agent:
    image: dockium-agent:${VERSION}
    container_name: dockium-agent
    restart: unless-stopped
    depends_on:
      - panel
    environment:
      TZ: "${TIMEZONE}"
      DOCKIUM_PANEL_URL: "http://panel:3443"
      DOCKIUM_AGENT_TOKEN: "\${DOCKIUM_AGENT_TOKEN}"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - dockium-agent-data:/data
EOF
  fi
  cat <<EOF
volumes:
  dockium-data:
EOF
  if [ -n "$AGENT_TOKEN" ]; then
    cat <<EOF
  dockium-agent-data:
EOF
  fi
} > "$INSTALL_DIR/docker-compose.yml"

chmod 600 "$INSTALL_DIR/docker-compose.yml" "$INSTALL_DIR/.env"

echo "==> Запуск"
(cd "$INSTALL_DIR" && $COMPOSE up -d)

echo
echo "Dockium запущен:"
echo "  Интерфейс:  http://<адрес-сервера>:3434"
echo "  Агенты:     <адрес-сервера>:3443"
echo
if [ -z "$AGENT_TOKEN" ]; then
  echo "Агент не запущен. Создайте агента в интерфейсе и выполните:"
  echo "  sudo DOCKIUM_AGENT_TOKEN='<токен>' $ROOT_DIR/install.sh"
  echo
fi
echo "Если панель доступна по домену, укажите его при повторном запуске:"
echo "  DOCKIUM_AGENT_ENDPOINT=panel.example.com:3443 DOCKIUM_AGENT_TOKEN='<токен>' $ROOT_DIR/install.sh"
