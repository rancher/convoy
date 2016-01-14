#!/bin/bash

cd $(dirname $0)

cat > template.go << EOF
package longhorn

const (
	DockerComposeTemplate = \`
$(<docker-compose.yml)
\`
)
EOF
