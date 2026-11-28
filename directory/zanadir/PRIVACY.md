# Privacy

This covers the zanadir plugin for Claude Code, the edition listed in
Anthropic's plugin directory.

**The plugin collects nothing.** It has no servers, accounts, analytics or
telemetry, and the MustacheCase maintainers receive no data from it.

**What runs, runs on your computer.** The `improve-ci` skill runs the zanadir
CLI in your project (`zanadir scan`, `zanadir fix`, `zanadir skills`) and reads
your files, each command with your approval. zanadir reads the project's files
locally, uploads nothing and has no telemetry. The skill doesn't ask it to read
anything over the network.

**Your conversation with Claude.** Files and command output that Claude reads
while using the skill become part of your Claude session. Anthropic's terms and
privacy policy cover that, not this plugin.

**Installing zanadir** happens outside the plugin, through whichever channel you
choose, under that channel's own terms.

Questions: [open an issue](https://github.com/MustacheCase/zanadir/issues).
