#!/bin/bash

# Script to build a Debian package for netConfigure
# Usage: ./build-deb.sh [version]

set -e

# Default version if not provided
VERSION=${1:-"0.0.1"}
PACKAGE_NAME="netconfigure"
MAINTAINER="Petr Boltik <petr.boltik@gmail.com>"
DESCRIPTION="Web frontend for SSH/SCP configuration tool"
ARCHITECTURE="amd64"

# Create a clean build environment
BIN_DIR="bin"
BUILD_DIR="build-deb"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Create the package directory structure
PKG_DIR="$BUILD_DIR/$PACKAGE_NAME-$VERSION"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/etc/default"
mkdir -p "$PKG_DIR/lib/systemd/system"

# Build the Go binary
echo "Building Go binary..."
CGO_ENABLED=0 go build -o "$BIN_DIR/$PACKAGE_NAME-$VERSION" main.go
CGO_ENABLED=0 go build -o "$PKG_DIR/usr/bin/$PACKAGE_NAME" main.go

# Create the DEBIAN control file
cat > "$PKG_DIR/DEBIAN/control" << EOF
Package: $PACKAGE_NAME
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCHITECTURE
Depends: netconfigure
Maintainer: $MAINTAINER
Description: $DESCRIPTION
 NetConfigure is tool for push configuration using SSH/SCP
EOF

# Create conffiles file to mark our config file to be preserved
cat > "$PKG_DIR/DEBIAN/conffiles" << EOF
/etc/default/$PACKAGE_NAME
EOF

# Create postinst script to set permissions and enable service
cat > "$PKG_DIR/DEBIAN/postinst" << EOF
#!/bin/bash
set -e

# Display schema import instructions
echo "  Configuration in file: /etc/default/netconfigure"
echo "  Start service using systemd: systemctl restart netconfigure"
echo "====================================================================="
echo ""

# Set permissions
chmod 755 /usr/bin/$PACKAGE_NAME

# Enable and start the service
systemctl daemon-reload
systemctl enable $PACKAGE_NAME.service
systemctl start $PACKAGE_NAME.service
if systemctl is-active --quiet $PACKAGE_NAME.service; then
        systemctl restart $PACKAGE_NAME.service
fi

exit 0
EOF

# Create postrm script to clean up on remove
cat > "$PKG_DIR/DEBIAN/postrm" << EOF
#!/bin/bash
set -e

if [ "\$1" = "remove" ]; then
    # Stop and disable service
    systemctl stop $PACKAGE_NAME.service || true
    systemctl disable $PACKAGE_NAME.service || true
    systemctl daemon-reload
fi

exit 0
EOF

# Make the DEBIAN scripts executable
chmod 755 "$PKG_DIR/DEBIAN/postinst"
chmod 755 "$PKG_DIR/DEBIAN/postrm"

# Create default configuration file
cat > "$PKG_DIR/etc/default/$PACKAGE_NAME" << EOF
# Default configuration for $PACKAGE_NAME

# Server configuration
LISTEN_ADDR=""
LISTEN_PORT="8080"
EOF

# Create systemd service file
cat > "$PKG_DIR/lib/systemd/system/$PACKAGE_NAME.service" << EOF
[Unit]
Description=NetConfigure - Web frontend for netConfigure

[Service]
Type=simple
WorkingDirectory=/usr/share/$PACKAGE_NAME
EnvironmentFile=/etc/default/$PACKAGE_NAME
ExecStart=/usr/bin/$PACKAGE_NAME \
  -listenAddr=\${LISTEN_ADDR} \
  -listenPort=\${LISTEN_PORT}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Build the package
echo "Building Debian package..."
dpkg-deb --build "$PKG_DIR"

# Move the resulting .deb file to the current directory
mv "$BUILD_DIR"/*.deb debian-pkg/.

echo "Package created: $PACKAGE_NAME-$VERSION.deb"
echo "You can install it with: sudo dpkg -i $PACKAGE_NAME-$VERSION.deb"
echo ""