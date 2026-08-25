class Zanadir < Formula
  desc "Scans CI/CD pipelines and suggests missing security and quality tools"
  homepage "https://github.com/MustacheCase/zanadir"
  url "https://github.com/MustacheCase/zanadir/archive/refs/tags/0.3.0.tar.gz"
  sha256 "1cf80d01ffdbc2951d6a3f022635e3c4448232396304f015451d5fd12af47196"
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
