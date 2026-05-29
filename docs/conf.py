"""Configuration file for the Sphinx documentation builder."""
import os
from pathlib import Path

external_projects_local_file = "projects.yaml"
external_projects_remote_repository = ""
#external_projects = ["k8s-device-plugin"]
external_projects = []
external_projects_current_project = "k8s-device-plugin"

html_baseurl = os.environ.get("READTHEDOCS_CANONICAL_URL", "instinct.docs.amd.com")
html_context = {}
if os.environ.get("READTHEDOCS", "") == "True":
    html_context["READTHEDOCS"] = True

project = "AMD Kubernetes Device Plugin Documentation"
version = "1.3.1"
release = version
html_title = f"Device Plugin Documentation {version}"
author = "Advanced Micro Devices, Inc."
copyright = "Copyright (c) 2025 Advanced Micro Devices, Inc. All rights reserved."

# Required settings
html_theme = "rocm_docs_theme"
html_theme_options = {
    "flavor": "instinct",
    "link_main_doc": True,
    "use_download_button": True,
}
extensions = ["rocm_docs"]

external_toc_path = "./sphinx/_toc.yml"

exclude_patterns = ['.venv']

html_extra_path = ["llms.txt"]

import re

EXCLUDED_DIRS = {
    "_build",
    "_templates",
    "_static",
    ".git",
    ".venv",
    "vendor",
}

MARKUP_PREFIXES = (
    ":::",
    "```{",
    "```",
    ":img-top:",
    ":class",
    ":link:",
    ":link-type:",
    ":shadow:",
    ":columns:",
    ":padding:",
    ":gutter:",
    ":open:",
    ":name:",
    ":header-rows:",
    ":alt:",
    "+++",
    "-->",
    "{bdg-",
)

# Matches lines like "align: center", "alt:", "name: foo" (directive options
# not starting with a colon, common in MyST figure/table fences)
_BARE_DIRECTIVE_RE = re.compile(r"^[a-z][a-z_-]*:\s*\S*$")

# Matches MyST/RST anchor labels like "(some-label)="
_ANCHOR_LABEL_RE = re.compile(r"^\(\w[\w-]*\)=$")

# Matches RST section underlines (e.g. "====", "----", "~~~~")
_RST_UNDERLINE_RE = re.compile(r"^[=\-~^\"\'#*+]{3,}$")

# Matches RST code block directives (e.g. ".. code-block:: cpp", ".. code:: sh")
_RST_CODE_BLOCK_RE = re.compile(r"^\.\.\s+(code-block|code|sourcecode)::")

# Matches markdown table separator rows (e.g. "|---|---|", "| :--- | ---: |").
_MD_TABLE_SEP_RE = re.compile(r"^\|[\s|:\-]+\|$")

# Matches RST directives whose indented body should be discarded (e.g. raw HTML).
_RST_SKIP_BLOCK_RE = re.compile(r"^\.\.\s+raw::")

# Matches HTML tags (e.g. "<div>", "</p>", "<!--") but NOT RST hyperlink URL
# continuation lines (e.g. "<https://...>`_").  The negative lookahead excludes
# URL schemes so that multi-line RST inline hyperlinks are preserved.
_HTML_TAG_RE = re.compile(r"^<(?!https?://|ftp://|mailto:)[a-zA-Z/!]")

# Matches trailing HTML close tags at the end of a prose line
# (e.g. "Browse blogs.</p>", "See the guide.</li></ul>").
_TRAILING_HTML_CLOSE_RE = re.compile(r"(</[a-zA-Z]+>)+\s*$")

MIN_PROSE_LINES = 10


def should_skip(path: Path) -> bool:
    return any(part in EXCLUDED_DIRS for part in path.parts)


def is_prose_line(line: str) -> bool:
    stripped = line.strip()
    if not stripped:
        return False
    if stripped.startswith(MARKUP_PREFIXES):
        return False
    # Drop bare directive-option lines (e.g. "align: center", "alt:")
    if _BARE_DIRECTIVE_RE.match(stripped):
        return False
    # Drop MyST/RST anchor labels (e.g. "(some-label)=")
    if _ANCHOR_LABEL_RE.match(stripped):
        return False
    # Drop markdown table separator rows (e.g. "|---|---|", "| :--- | ---: |")
    if _MD_TABLE_SEP_RE.match(stripped):
        return False
    # Drop HTML tags (e.g. "<div>", "</p>") but keep RST hyperlink URL
    # continuation lines (e.g. "<https://rocm.docs.amd.com/...>`_")
    if _HTML_TAG_RE.match(stripped):
        return False
    # Drop RST directives, comments, hyperlink targets, and substitution definitions
    if stripped.startswith(".."):
        return False
    # Drop YAML frontmatter key-value pairs (e.g. "description lang=en": "text")
    if stripped.startswith('"') and re.match(r'^"[^"]+"\s*:', stripped):
        return False
    # Drop RST field list items (e.g. ":type: int") and extended RST meta
    # options (e.g. ":description lang=en: text"). Excludes inline roles at line
    # start (e.g. ":cpp:func:`hipMalloc` returns..." or ":ref:`foo <bar>` describes...")
    # because those are followed by a backtick, not a space or end-of-line.
    if re.match(r"^:[A-Za-z][A-Za-z0-9_ =-]*:(\s|$)", stripped):
        return False
    # Drop RST section underlines (e.g. "====", "----", "~~~~")
    if _RST_UNDERLINE_RE.match(stripped):
        return False
    return True


def generate_combined_markdown(app, exception):
    if exception:
        return

    docs_root = Path(app.srcdir)
    output_file = Path(app.outdir) / "llms-full.txt"
    base_file = docs_root / "llms.txt"

    combined = []

    if base_file.exists():
        base_text = base_file.read_text(encoding="utf-8").rstrip().rstrip("-").rstrip()
        combined.append(base_text)
    else:
        combined.append("# AMD GPU Device Plugin for Kubernetes")

    all_files = sorted(
        list(docs_root.rglob("*.md")) + list(docs_root.rglob("*.rst"))
    )

    for doc_file in all_files:
        if should_skip(doc_file):
            continue

        if doc_file == base_file:
            continue

        try:
            content = doc_file.read_text(encoding="utf-8")
        except Exception:
            continue

        lines = content.splitlines()
        prose_lines = [line for line in lines if is_prose_line(line)]

        if len(prose_lines) < MIN_PROSE_LINES:
            continue

        relative = doc_file.relative_to(docs_root)
        in_backtick_fence = False
        in_rst_code_block = False
        in_rst_skip_block = False
        in_html_comment = False  # inside <!-- ... --> block
        in_html_open_tag = False  # inside a multi-line HTML opening tag
        kept = []
        for line in lines:
            stripped = line.strip()
            # Backtick fences (MyST/Markdown)
            if stripped.startswith("```"):
                in_backtick_fence = not in_backtick_fence
                kept.append(line)
                continue
            if in_backtick_fence:
                kept.append(line)
                continue
            # HTML comment block (<!-- ... -->): discard all content until -->
            if in_html_comment:
                if "-->" in stripped:
                    in_html_comment = False
                continue
            # RST skip block (e.g. .. raw::): discard all indented content
            if in_rst_skip_block:
                if not stripped or line[0] in (" ", "\t"):
                    continue
                in_rst_skip_block = False
            # RST code block: exit when a non-blank, non-indented line appears
            if in_rst_code_block:
                if not stripped or line[0] in (" ", "\t"):
                    kept.append(line)
                    continue
                in_rst_code_block = False
            # RST raw block: enter and discard both the directive and its body
            if _RST_SKIP_BLOCK_RE.match(stripped):
                in_rst_skip_block = True
                continue
            # RST code block: enter on directive line (directive itself is dropped)
            if _RST_CODE_BLOCK_RE.match(stripped):
                in_rst_code_block = True
                continue
            # HTML comment open (<!-- ... -->): discard opener and enter state
            if stripped.startswith("<!--"):
                if "-->" not in stripped:
                    in_html_comment = True
                continue
            # Multi-line HTML opening tag: skip continuation lines until >
            if in_html_open_tag:
                if ">" in stripped:
                    in_html_open_tag = False
                continue
            # Detect HTML opening tags that wrap across lines (no > on this line)
            if _HTML_TAG_RE.match(stripped) and ">" not in stripped:
                in_html_open_tag = True
                continue
            if not stripped:
                kept.append(line)
            elif is_prose_line(line):
                # Strip trailing HTML close tags (e.g. "See the guide.</p>")
                cleaned = _TRAILING_HTML_CLOSE_RE.sub("", line).rstrip()
                kept.append(cleaned if cleaned.strip() else line)
        cleaned = "\n".join(kept)

        combined.append(f"\n\n---\n\n# {relative}\n")
        combined.append(cleaned.strip())

    output_file.write_text(
        "\n".join(combined) + "\n",
        encoding="utf-8",
    )

def setup(app):
    app.connect("build-finished", generate_combined_markdown)
