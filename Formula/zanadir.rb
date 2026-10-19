class Zanadir < Formula
  desc "Scans CI/CD pipelines and suggests missing security and quality tools"
  homepage "https://github.com/MustacheCase/zanadir"
  url "https://github.com/MustacheCase/zanadir/archive/refs/tags/0.14.1.tar.gz"
  sha256 "67e54e6d28adf1231c3df6933552ecec729d719862db7e9240420ec81d5139c3"
  license "MIT"
  head "https://github.com/MustacheCase/zanadir.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "zanadir", shell_output("#{bin}/zanadir --help")

    (testpath/".github/workflows").mkpath
    (testpath/".github/workflows/ci.yml").write <<~YAML
      name: ci
      on: [push]
      jobs:
        build:
          runs-on: ubuntu-latest
          steps:
            - run: go test ./...
    YAML
    system "git", "init"

    output = shell_output("#{bin}/zanadir scan --dir #{testpath} --output json")
    assert_match "\"ID\"", output
  end
end
