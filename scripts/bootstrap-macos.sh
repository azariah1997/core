#!/usr/bin/env bash
set -euo pipefail
if ! command -v brew >/dev/null 2>&1; then
  echo "Install Homebrew first from brew.sh" >&2
  exit 1
fi
brew update
brew install git go node python terraform kubectl helm
brew install --cask docker flutter android-studio

echo "Install Xcode from the Mac App Store, then run: sudo xcodebuild -license accept"
echo "Start Docker Desktop, then from the repo: cp .env.example .env && make local-up"
