#!/usr/bin/env bash
# Erzeugt die Release Notes einer Version aus CHANGELOG.md.
#   .github/scripts/release-notes.sh 0.3.1 [owner/repo]
# Bricht ab, wenn die Version keinen Abschnitt im Changelog hat.
set -euo pipefail

version="${1#v}"
repo="${2:-mrcdlm/dnsdeck}"
owner="${repo%%/*}"

notes="$(awk -v v="$version" '
  index($0, "## [" v "]") == 1 { found = 1; next }
  found && /^## \[/ { exit }
  found { print }
' CHANGELOG.md | sed -e '/./,$!d' |   # führende Leerzeilen entfernen
  # Eingerückte Fortsetzungszeilen an den Listenpunkt anhängen – GitHub zeigt
  # in Release-Texten jeden Zeilenumbruch an.
  awk '/^  [^ -]/ && have && out != "" { sub(/^ +/, ""); out = out " " $0; next }
       { if (have) print out; out = $0; have = 1 }
       END { if (have) print out }')"

if [ -z "$notes" ]; then
  echo "CHANGELOG.md has no section for $version" >&2
  exit 1
fi

cat <<NOTES
$notes

---

### Install or update

Image: \`ghcr.io/$owner/dnsdeck:$version\` (linux/amd64, linux/arm64)

Set \`DNSDECK_VERSION=$version\` in \`.env\`, then run:

\`\`\`sh
docker compose pull && docker compose up -d
\`\`\`

New here? Start with the [quick start](https://github.com/$repo#quick-start).
NOTES
