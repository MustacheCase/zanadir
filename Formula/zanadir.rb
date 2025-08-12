class Zanadir < Formula
  desc "A tool for directory navigation and management"
  homepage "https://github.com/MustacheCase/zanadir"
  license "MIT"

  # Get the latest version from git tags
  version = "0.1.1"
  
  stable do
    url "https://github.com/MustacheCase/zanadir/archive/refs/tags/#{version}.tar.gz"
    sha256 "84165bcdc12ff56058ff438fe7cbbdd3d694c24bb9f4d2f546184ce83ca0adbc"
    version version
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