gogcli security patches - suggested changes

This folder contains suggested fixes for high-severity findings reported by semgrep.

1) Avoid github context interpolation in GitHub Actions run steps
 - Change `run:` steps that use `${{ github.* }}` directly to use `env:` variables and reference them safely.
 - Example change (in .github/workflows/release.yml):

   # BAD
   run: echo "Releasing ${{ github.ref }}" && ./release.sh

   # GOOD
   env:
     GITHUB_REF: "${{ github.ref }}"
   run: |
     echo "Releasing \"$GITHUB_REF\""
     ./release.sh

2) Sanitize exec.Command inputs
 - Avoid passing unchecked user input to exec.Command. Validate against an allowlist or construct fixed arguments.
 - Example fix: if opening a browser with a URL, ensure the URL is validated and not directly concatenated into a shell command.

3) Open redirect mitigation
 - For endpoints that redirect to user-provided URLs, implement an allowlist of domains or only permit relative paths.

4) ResponseWriter XSS mitigation
 - Use html/template for rendering and ensure values are escaped.

For each recommended change below there is a suggested diff file (UNAPPLIED) and a short explanation.