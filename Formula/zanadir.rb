class Zanadir < Formula
  desc "A tool for directory navigation and management"
  homepage "https://github.com/MustacheCase/zanadir"
  license "MIT"

  stable do
    url "https://github.com/MustacheCase/zanadir/archive/refs/tags/0.0.5.tar.gz"
    sha256 "9a54d970ee594f21395f0de3210bce8c0059a48d2cf84911207baad125cb9a13"
    version "0.0.5"
  end

  head "https://github.com/MustacheCase/zanadir.git", branch: "main"

  # Note: If the project requires specific Go dependencies at build time,
  # use go_resource blocks to specify them. Example:
  # go_resource "github.com/some/dependency" do
  #   url "https://github.com/some/dependency.git",
  #       :tag => "v1.0.0"
  # end
  # This ensures consistent builds across different environments.

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  # Test verifies that the binary was built correctly and can execute basic commands
  # by checking if the help command works, which indicates the CLI is properly
  # initialized and can process commands.
  test do
    system "#{bin}/zanadir", "--help"
  end
end 