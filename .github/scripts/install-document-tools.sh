#!/usr/bin/env bash
# Run as root on an ephemeral Ubuntu CI runner. Download before invoking dpkg,
# so a stalled mirror can be retried without interrupting package configuration.
set -euo pipefail

options=(-o Acquire::Retries=1 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20)
packages=(tesseract-ocr tesseract-ocr-eng antiword libreoffice-writer-nogui)

download() {
  timeout --kill-after=5s 30s apt-get "${options[@]}" -o APT::Update::Error-Mode=any update -q &&
    timeout --kill-after=5s 90s apt-get "${options[@]}" install -y -q --no-install-recommends --download-only "${packages[@]}"
}

if ! download; then
  echo '::warning::Document-tool download failed or exceeded its time limit; retrying with the Ubuntu archive mirror.'
  for sources in /etc/apt/apt-mirrors.txt /etc/apt/sources.list \
    /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; do
    if [[ -f "$sources" ]]; then
      sed -i 's|://azure[.]archive[.]ubuntu[.]com/ubuntu|://archive.ubuntu.com/ubuntu|g' "$sources"
    fi
  done
  download
fi

apt-get "${options[@]}" install -y -q --no-install-recommends --no-download "${packages[@]}"
