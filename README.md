# Terraform/OpenTofu Provider: Ghost Inspector

Manage [Ghost Inspector](https://ghostinspector.com) suites, tests, steps, and suite variables as code. Works with both Terraform and OpenTofu.

Built for teams whose Ghost Inspector suites outgrew point-and-click management: every suite, test, and step is reviewable in Git, adopted without churn, and drift is visible in `plan` output before anything changes.

```hcl
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {
  # or export GHOSTINSPECTOR_API_KEY
  api_key = var.ghostinspector_api_key
}

resource "ghostinspector_folder" "app" {
  name = "My App"
}

resource "ghostinspector_suite" "release" {
  name      = "Automated Tests"
  folder_id = ghostinspector_folder.app.id
  browser   = "chrome"
  region    = "us-east-1"
}

resource "ghostinspector_test" "homepage" {
  suite_id = ghostinspector_suite.release.id
  name     = "Homepage loads"

  steps = [
    { command = "open", value = "/" },
    { command = "assertElementPresent", target = ".hero" },
  ]
}
```

## Why this provider exists

Ghost Inspector's API cannot update test steps in place. Every workaround (UI editing, export/import scripts, `null_resource` + curl modules) loses one or more of: state, drift detection, history of what changed, or the wiring between shared modules and the tests that execute them.

This provider models the API honestly:

- **Steps are managed state.** Read normalizes the API's step shapes (string targets vs selector-candidate arrays, empty vs absent fields), so `plan` only shows real changes.
- **Step changes replace the test.** The API only supports delete + re-import, so that is what a step change does: export, overlay your managed fields, delete, re-import, with a best-effort restore if the replacement import fails. The replacement gets a new test ID; run history stays with the removed test (an API limitation no tool can avoid).
- **Module wiring survives replacement.** Parent tests reference shared modules as `value = ghostinspector_test.login.id` inside an `execute` step. When a module is replaced, the graph sees the new ID and re-imports the parents with updated references. No dangling `execute` steps, ever.
- **Existing suites adopt without churn.** On create, folders, suites, and tests are matched by name (or pinned by `suite_id` on the suite resource): if the object already exists it is adopted into state and only drift is corrected. You do not need 37 `terraform import` commands to start managing a live suite.
- **Private values stay private.** Private suite variables are masked by the API on read; the provider carries the configured value in state instead of showing perpetual diffs. Private step flags (`private = true` on a password `assign`) are modeled and preserved.

## Resources and data sources

| Name | Kind | Purpose |
| --- | --- | --- |
| `ghostinspector_folder` | resource | Folder. Adopts by name. (The API cannot delete folders; destroy only removes state.) |
| `ghostinspector_suite` | resource | Suite + default test settings + schedule. Adopts by name. |
| `ghostinspector_suite_variables` | resource | The complete variable set of one suite (the API replaces the whole set). |
| `ghostinspector_test` | resource | One test, metadata + steps. Adopts by name. |
| `ghostinspector_folder` | data source | Look up a folder by `id` or `name`. |

Full reference: [docs/](docs/index.md) (also rendered on the registry page).

## Importing existing objects

Everything also supports classic import:

```bash
tofu import ghostinspector_suite.release 6583999a6e489be528f1729a
tofu import ghostinspector_test.homepage 6a8a199960825c6d59fa081e
tofu import ghostinspector_suite_variables.release 6583999a6e489be528f1729a
```

For suites with many tests, the adopt-on-create flow is usually easier: write the config with matching names, apply, and the provider warns about each adoption instead of creating duplicates.

## Semantics worth knowing

- **Null means unmanaged.** Every settings attribute (`browser`, `max_wait_delay`, `user_agent`, ...) is optional. Null config mirrors whatever the API holds, so you can manage exactly the fields you care about and leave the rest to the UI.
- **Null `steps` leaves steps unmanaged** for that test (metadata only).
- **Test IDs change on step replacement.** Anything referencing `ghostinspector_test.x.id` is updated by the graph; anything holding the ID as a literal string is not. Use references.
- **Suite variables are all-or-nothing** per suite, matching the API. Manage a suite's variables in exactly one `ghostinspector_suite_variables` resource.
- **Organization resolution.** The API requires an explicit organization on create and derives it incorrectly for keys that can see several organizations. The provider resolves the organization from the target folder, or tries every visible organization until one accepts the write. Set `organization` on the folder resource to pin it.

## Development

```bash
go build ./...
go test ./internal/gi/          # unit tests

# acceptance tests (real API, creates a scratch folder + suites)
TF_ACC=1 \
TF_ACC_TERRAFORM_PATH=$(which tofu) \
TF_ACC_PROVIDER_NAMESPACE=udx \
TF_ACC_PROVIDER_HOST=registry.opentofu.org \
GHOSTINSPECTOR_API_KEY=... \
go test ./internal/provider/ -timeout 20m
```

Note: the Ghost Inspector API cannot delete folders, so the acceptance suite reuses one persistent folder (`terraform-provider-ghostinspector-acc`) and deletes the suites it creates.

## License

[Mozilla Public License 2.0](LICENSE)
