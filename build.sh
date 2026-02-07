#!/bin/bash
# Build script for Linux amd64 using Docker
set -e

DIST_DIR="dist"
BINARY_NAME="ocserv-exporter"

echo "=== Building ocserv-exporter for Linux amd64 ==="

# Build binary in Docker
# Using Debian Bullseye (glibc 2.31) for compatibility with Ubuntu 20.04+
docker run --rm --platform linux/amd64 -v "$(pwd)":/app -w /app golang:1.23-bullseye \
    bash -c "apt-get update && apt-get install -y libsystemd-dev && go mod tidy && go build -o ${BINARY_NAME}-linux-amd64 ."

echo "Build completed: ${BINARY_NAME}-linux-amd64"

# Create dist directory structure
echo "=== Preparing dist folder ==="
mkdir -p "${DIST_DIR}/grafana"
mkdir -p "${DIST_DIR}/prometheus"

# Copy binary
mv "${BINARY_NAME}-linux-amd64" "${DIST_DIR}/${BINARY_NAME}"
chmod +x "${DIST_DIR}/${BINARY_NAME}"

# Copy config files
cp systemd/ocserv-exporter.service "${DIST_DIR}/"
cp grafana/dashboard.json "${DIST_DIR}/grafana/"
cp prometheus/alerts.yml "${DIST_DIR}/prometheus/"
cp prometheus/scrape_config.yml "${DIST_DIR}/prometheus/"
cp README.md "${DIST_DIR}/"

# Create install script
cat > "${DIST_DIR}/install.sh" << 'EOF'
#!/bin/bash
set -e

INSTALL_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"
SERVICE_USER="ocserv-exporter"

echo "Installing ocserv-exporter..."

# Create service user if doesn't exist
if ! id "${SERVICE_USER}" &>/dev/null; then
    echo "Creating user ${SERVICE_USER}..."
    sudo useradd -r -s /sbin/nologin -G systemd-journal "${SERVICE_USER}"
else
    # Ensure user is in systemd-journal group
    sudo usermod -aG systemd-journal "${SERVICE_USER}"
fi

# Copy binary
sudo cp ocserv-exporter "${INSTALL_DIR}/"
sudo chmod +x "${INSTALL_DIR}/ocserv-exporter"

# Copy systemd service
sudo cp ocserv-exporter.service "${SERVICE_DIR}/"

# Reload systemd
sudo systemctl daemon-reload

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit service file if needed: sudo vim ${SERVICE_DIR}/ocserv-exporter.service"
echo "  2. Enable and start: sudo systemctl enable --now ocserv-exporter"
echo "  3. Check status: sudo systemctl status ocserv-exporter"
echo "  4. View metrics: curl localhost:9617/metrics"
echo ""
echo "Prometheus setup:"
echo "  Add to prometheus.yml scrape_configs (see prometheus/scrape_config.yml):"
echo "    - job_name: 'ocserv'"
echo "      static_configs:"
echo "        - targets: ['localhost:9617']"
echo ""
echo "Optional:"
echo "  - Import grafana/dashboard.json to Grafana"
echo "  - Add prometheus/alerts.yml for alerting rules"
EOF
chmod +x "${DIST_DIR}/install.sh"

echo ""
echo "=== Done ==="
echo "Distribution package ready in: ${DIST_DIR}/"
ls -la "${DIST_DIR}/"
