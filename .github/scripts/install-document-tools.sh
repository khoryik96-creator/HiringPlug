#!/usr/bin/env bash
# Run as root on an ephemeral Ubuntu CI runner. Download before invoking dpkg,
# so a stalled mirror can be retried without interrupting package configuration.
set -euo pipefail

options=(-o Acquire::Retries=1 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20)
archive_dir=${1:-/var/cache/apt/archives}
mkdir -p "$archive_dir/partial"
options+=(-o "Dir::Cache::archives=$archive_dir" -o APT::Keep-Downloaded-Packages=true)
mapfile -t packages < "$(dirname -- "${BASH_SOURCE[0]}")/document-tools.packages"

download() {
  timeout --kill-after=5s 30s apt-get "${options[@]}" -o APT::Update::Error-Mode=any update -q &&
    timeout --kill-after=5s 90s apt-get "${options[@]}" install -y -q --no-install-recommends --download-only "${packages[@]}"
}

if ! download; then
  echo '::warning::Document-tool download failed or exceeded its time limit; checking the Ubuntu mirror before retrying.'
  switched=false
  for sources in /etc/apt/apt-mirrors.txt /etc/apt/sources.list \
    /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; do
    if [[ -f "$sources" ]] && grep -Eq 'https?://([[:alnum:]-]+[.])+archive[.]ubuntu[.]com/ubuntu' "$sources"; then
      sed -E -i 's|https?://([[:alnum:]-]+[.])+archive[.]ubuntu[.]com/ubuntu|http://archive.ubuntu.com/ubuntu|g' "$sources"
      switched=true
    fi
  done
  if "$switched"; then
    echo 'Retrying with archive.ubuntu.com.'
  else
    echo '::warning::No supported Ubuntu mirror URL was changed; retrying the configured sources once.'
  fi
  download
fi

apt-get "${options[@]}" install -y -q --no-install-recommends --no-download "${packages[@]}"
