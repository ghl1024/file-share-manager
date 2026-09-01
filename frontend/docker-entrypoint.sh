#!/bin/sh
# - Copyright (c) 2026 HaydenGuo
# - Project: file-share-manager
# - Gitee: https://gitee.com/ghl1024/file-share-manager
# - GitHub: https://github.com/ghl1024/file-share-manager
# - CNB: https://cnb.cool/ghl1024/file-share-manager
# - GitCode: https://gitcode.com/haydenguo/file-share-manager
# - Author: https://hayden.pub

set -eu

version="${FILESHARE_VERSION:-v0.0.0}"
commit="${FILESHARE_GIT_COMMIT:-none}"
build_time="${FILESHARE_BUILD_TIME:-unknown}"

if [ -z "$version" ] || [ "$version" = "unknown" ] || [ "$version" = "none" ]; then
  version="v0.0.0"
fi
case "$version" in
  v*) ;;
  *) version="v$version" ;;
esac

if [ -z "$commit" ] || [ "$commit" = "unknown" ]; then
  commit="none"
fi
if [ -z "$build_time" ] || [ "$build_time" = "unknown" ]; then
  build_time="$(date '+%Y-%m-%d %H:%M:%S')"
fi

cat <<'EOF'
**************************************************************************
**************************************************************************
  _____ _ _      _____ _                    __  __
 |  ___(_) | ___/  ___| |__   __ _ _ __ ___|  \/  | __ _ _ __
 | |_  | | |/ _ \___ \| '_ \ / _' | '__/ _ \ |\/| |/ _' | '_ |
 |  _| | | |  __/___) | | | | (_| | | |  __/ |  | | (_| | | | |
 |_|   |_|_|\___|____/|_| |_|\__,_|_|  \___|_|  |_|\__,_|_| |_|

  File Share Manager - 开源工作空间文件共享服务
EOF
printf '  Version:   %s\n' "$version"
printf '  Runtime:   nginx\n'
printf '  Build:     %s\n' "$build_time"
printf '  Commit:    %s\n' "$commit"
cat <<'EOF'
  Author:    https://hayden.pub
  Blog:      https://hayden.pub
  GitHub:    https://github.com/ghl1024/file-share-manager
  Gitee:     https://gitee.com/ghl1024/file-share-manager
  CNB:       https://cnb.cool/ghl1024/file-share-manager
  License:   Apache-2.0
  Listen:    0.0.0.0:80
**************************************************************************
**************************************************************************
EOF

exec nginx -g 'daemon off;'
