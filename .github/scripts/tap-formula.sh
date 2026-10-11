#!/bin/sh
# usage: tap-formula.sh <tag> <sha256 of the tag's source tarball>
# Prints Formula/sqlmux.rb for kackerx/homebrew-tap (release.yml, tech-design
# §15「发布与分发」): built from the tag's source with Go, macOS and Linux alike.
set -eu
tag=$1 sha=$2
cat <<RUBY
class Sqlmux < Formula
  desc "Terminal database client organized like tmux, for PostgreSQL and MySQL"
  homepage "https://github.com/kackerx/sqlmux"
  url "https://github.com/kackerx/sqlmux/archive/refs/tags/${tag}.tar.gz"
  sha256 "${sha}"
  license "MIT"
  head "https://github.com/kackerx/sqlmux.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-X main.version=v#{version}" # std_go_args adds -s -w
    system "go", "build", *std_go_args(ldflags:), "./cmd/sqlmux"
  end

  test do
    assert_match "sqlmux v#{version}", shell_output("#{bin}/sqlmux --version")
  end
end
RUBY
