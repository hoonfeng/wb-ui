import json, os, sys, urllib.request, re

url = "http://localhost:9090"
html = urllib.request.urlopen(url, timeout=10).read().decode('utf-8')

# Extract script sources
scripts = re.findall(r'<script[^>]*src=["\']([^"\']+)["\']', html)
print("Scripts:", scripts)

# Output index.html
out_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "output")
os.makedirs(out_dir, exist_ok=True)
with open(os.path.join(out_dir, "full_browser_dom.json"), "w") as f:
    # We'll populate this from the browser
    pass

print("HTML length:", len(html))
print("Title:", re.search(r'<title>([^<]+)</title>', html).group(1) if re.search(r'<title>([^<]+)</title>', html) else "N/A")
