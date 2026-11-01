"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const form = $("form"), input = $("repo"), button = $("scan"), status = $("status");

  const say = (text, isError) => {
    status.textContent = text;
    status.classList.toggle("error", Boolean(isError));
  };
  const el = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text !== undefined) node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  const link = (href, text) => {
    const a = el("a", text);
    if (/^https?:\/\//.test(href)) a.href = href;
    return a;
  };

  // The module ships gzipped (4 MB rather than 17), since GitHub Pages may
  // not compress it. A server that already decoded it sends plain wasm.
  // Bumped with each zanadir-action release.
  const ACTION = "mustachecase/zanadir-action@v1.37";
  const WORKFLOW = `name: zanadir

on:
  push:
  pull_request:

permissions: {}

jobs:
  scan:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - uses: ${ACTION}
        with:
          dir: .

  # Publishes the score for the README badge, from the default branch only.
  badge:
    if: github.event_name == 'push' && github.ref_name == github.event.repository.default_branch
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v7
      - uses: ${ACTION}
        with:
          dir: .
          badge: zanadir-badge.json
      - name: Publish the badge when it moved
        run: |
          set -euo pipefail
          cp zanadir-badge.json "$RUNNER_TEMP/zanadir-badge.json"
          rm -f zanadir-badge.json
          git config user.name github-actions
          git config user.email github-actions@github.com
          if git fetch --depth=1 origin badges 2>/dev/null; then
            git switch --discard-changes -C badges FETCH_HEAD
          else
            git switch --orphan badges
          fi
          cp "$RUNNER_TEMP/zanadir-badge.json" zanadir-badge.json
          git add zanadir-badge.json
          if git diff --cached --quiet; then exit 0; fi
          git commit -m "chore: update the zanadir badge"
          git push origin badges
`;

  // The colours zanadir --badge writes, so the preview matches the badge.
  const colourOf = ({ covered, total }) => {
    if (total === 0) return "lightgrey";
    if (covered === total) return "brightgreen";
    const percent = Math.floor((covered * 100) / total);
    return percent >= 80 ? "green" : percent >= 60 ? "yellow" : percent >= 40 ? "orange" : "red";
  };

  const go = new Go();
  const loading = (async () => {
    const response = await fetch("zanadir.wasm.gz");
    if (!response.ok) throw new Error("Couldn't load zanadir (" + response.status + ").");
    let bytes = new Uint8Array(await response.arrayBuffer());
    if (!(bytes[0] === 0 && bytes[1] === 0x61 && bytes[2] === 0x73 && bytes[3] === 0x6d)) {
      const plain = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"));
      bytes = new Uint8Array(await new Response(plain).arrayBuffer());
    }
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(instance);
    $("version").textContent = globalThis.zanadir.version;
  })();

  // owner/name from what was typed, or null; zanadir checks it properly.
  const repoOf = (text) => {
    const m = text.trim().match(/^(?:https?:\/\/)?(?:www\.)?(?:github\.com\/)?([A-Za-z0-9-]+\/[A-Za-z0-9._-]+?)(?:\.git)?(?:[/?#].*)?$/);
    return m ? m[1] : null;
  };

  const explain = (message) => {
    if (/ 404 /.test(message)) return "That repository wasn't found. The page reads public repositories only.";
    if (/ (403|429) /.test(message)) return "GitHub's limit of 60 requests an hour without signing in is used up. Try again later, or run the CLI with a token.";
    if (/truncated|more than \d+ files/.test(message)) return "This repository is too large to scan in the browser. The CLI can scan it: zanadir scan --repo " + input.value.trim();
    if (/Failed to fetch|fetch\(\) failed/.test(message)) return "Couldn't reach GitHub. Check your connection and try again.";
    return message;
  };

  const render = (repo, report) => {
    const all = globalThis.zanadir.categories;
    const uncovered = new Set(report.uncovered.map((c) => c.id));
    const na = new Set(report.notApplicable);
    const weak = new Set(report.weakened.map((w) => w.category));

    $("covered").textContent = report.score.covered;
    $("total").textContent = report.score.total;
    $("repo-name").textContent = repo;
    $("platforms").textContent = report.platforms.length
      ? "CI read: " + report.platforms.join(", ")
      : "No CI configuration found";
    $("fix-cmd").textContent = "zanadir fix --repo " + repo;

    const resultURL = location.origin + location.pathname + "?repo=" + repo;
    const linkBadge = "https://img.shields.io/badge/zanadir-scan_this_CI-b4441f";
    $("link-badge").src = linkBadge;
    $("link-md").textContent = "[![zanadir](" + linkBadge + ")](" + resultURL + ")";
    $("workflow").textContent = WORKFLOW;
    const score = report.score;
    $("score-badge").src = "https://img.shields.io/badge/zanadir-" +
      encodeURIComponent(score.covered + "/" + score.total) + "-" + colourOf(score);
    $("score-badge").alt = "zanadir " + score.covered + "/" + score.total;
    $("score-md").textContent = "[![zanadir](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/" +
      repo + "/badges/zanadir-badge.json)](" + resultURL + ")";

    const weakened = $("weakened");
    weakened.replaceChildren();
    for (const w of report.weakened) {
      const li = el("li");
      const head = el("div", undefined, "row-head");
      head.append(el("b", w.category), el("span", w.verdict, "verdict " + w.verdict));
      li.append(head);
      if (w.location) {
        const where = el("p", undefined, "detail");
        const path = w.location.split("/").map(encodeURIComponent).join("/");
        where.append(link("https://github.com/" + repo + "/blob/HEAD/" + path, w.location));
        li.append(where);
      }
      if (w.reason) li.append(el("p", w.reason, "detail"));
      weakened.append(li);
    }
    $("weakened-block").hidden = report.weakened.length === 0;

    const missing = $("uncovered");
    missing.replaceChildren();
    for (const c of report.uncovered) {
      const li = el("li");
      const head = el("div", undefined, "row-head");
      head.append(el("b", c.id));
      li.append(head, el("p", c.description, "detail"));
      if (c.suggestions && c.suggestions.length) {
        const more = el("details");
        more.append(el("summary", "Tools that cover it"));
        const list = el("ul");
        for (const s of c.suggestions.slice(0, 6)) {
          const item = el("li");
          item.append(link(s.repository, s.name), document.createTextNode(": " + s.description));
          list.append(item);
        }
        more.append(list);
        li.append(more);
      }
      missing.append(li);
    }
    $("uncovered-block").hidden = report.uncovered.length === 0;

    const covered = all.filter((name) => !uncovered.has(name) && !na.has(name));
    $("covered-list").replaceChildren(...covered.map((name) =>
      el("span", weak.has(name) ? name + " (can't block)" : name, weak.has(name) ? "chip weak" : "chip")));
    $("covered-block").hidden = covered.length === 0;
    $("na-list").replaceChildren(...[...na].map((name) => el("span", name, "chip")));
    $("na-block").hidden = na.size === 0;

    $("result").hidden = false;
  };

  const scan = async (text) => {
    const repo = repoOf(text);
    if (!repo) {
      say("Enter a repository as owner/name, for example MustacheCase/zanadir.", true);
      return;
    }
    button.disabled = true;
    $("result").hidden = true;
    const started = performance.now();
    say("Loading zanadir (4 MB, once)…");
    try {
      await loading;
      say("Reading " + repo + " from GitHub…");
      const report = JSON.parse(await globalThis.zanadirScan(repo, "json"));
      render(repo, report);
      say("Scanned in " + ((performance.now() - started) / 1000).toFixed(1) + " s.");
      // repoOf allows only characters that need no escaping, slash included.
      history.replaceState(null, "", location.pathname + "?repo=" + repo);
    } catch (err) {
      say(explain(String(err && err.message || err)), true);
    } finally {
      button.disabled = false;
    }
  };

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    scan(input.value);
  });
  for (const copy of document.querySelectorAll("button.copy")) {
    copy.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText($(copy.dataset.copy).textContent);
        copy.textContent = "Copied";
      } catch {
        copy.textContent = "Select and copy";
      }
      setTimeout(() => { copy.textContent = "Copy"; }, 2000);
    });
  }

  $("share").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(location.href);
      say("Link copied.");
    } catch {
      say("Copy the address bar to share this result.");
    }
  });

  const asked = new URLSearchParams(location.search).get("repo");
  if (asked) {
    input.value = asked;
    scan(asked);
  }
})();
