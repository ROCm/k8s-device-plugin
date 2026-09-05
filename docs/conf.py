"""Configuration file for the Sphinx documentation builder."""
import os

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
    "flavor": "instinct-design",
    "link_main_doc": True,
    "use_download_button": True,
}
extensions = ["rocm_docs"]

external_toc_path = "./sphinx/_toc.yml"

exclude_patterns = ['.venv']


# Generate llms.txt and llms-full.txt after each build (the llms.txt standard,
# https://llmstxt.org/). See the rocm-docs-core guide:
# https://rocm.docs.amd.com/projects/rocm-docs-core/en/latest/user_guide/llms.html
rocm_docs_generate_llms = True
