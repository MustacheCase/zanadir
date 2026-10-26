class Zanadir < Formula
  desc "Find missing and non-blocking security controls in CI pipelines"
  homepage "https://github.com/MustacheCase/zanadir"
  url "https://github.com/MustacheCase/zanadir/archive/refs/tags/0.23.0.tar.gz"
  sha256 "4550268922a4a3d70b50e8c0cb84cf0f8c7269072a5a635207e63f034f6bdad2"
  license "MIT"
  head "https://github.com/MustacheCase/zanadir.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/MustacheCase/zanadir/app.version=#{version}")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/zanadir --version")

    (testpath/".github/workflows/ci.yml").write <<~YAML
      name: ci
      on: [pull_request]
      jobs:
        test:
          runs-on: ubuntu-latest
          steps:
            - run: go test ./...
    YAML
    system "git", "init"

    report = JSON.parse(shell_output("#{bin}/zanadir scan --dir #{testpath} --output json"))
    assert_equal ["github"], report["platforms"]
    assert_includes report["uncovered"].map { |c| c["id"] }, "SAST"
  end
end
