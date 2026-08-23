# ghostinspector_suite_variables

Manages the complete variable set of one suite. The Ghost Inspector API replaces the entire set on every update, so this resource owns every variable on the suite: a variable that exists on the suite but not in this resource is removed on the next apply.

Private variable values are masked by the API on read. The provider keeps the configured value in state, so private values do not produce perpetual diffs. Values are marked sensitive.

## Example

```hcl
resource "ghostinspector_suite_variables" "release" {
  suite_id = ghostinspector_suite.release.id

  variables = [
    { name = "siteUrl", value = "https://staging.example.com" },
    { name = "defaultUser", value = "gi-test@example.com" },
    { name = "defaultUserPassword", value = var.gi_test_password, private = true },
  ]
}
```

## Argument reference

- `suite_id` (string, required) - Managed suite. Changing it replaces the resource.
- `variables` (set of objects, required) - Complete variable set:
  - `name` (string, required) - Referenced in tests as `{{name}}`.
  - `value` (string, sensitive) - May contain `{{otherVariable}}` references.
  - `private` (bool, default false) - Mask the value in the UI and API.

## Removal

Removing this resource from configuration only removes it from state; the variable set is left on the suite. This matches the "omitting variables leaves them unmanaged" contract. To clear a suite's variables, apply an empty `variables = []` first.

## Import

Import with the suite ID:

```bash
tofu import ghostinspector_suite_variables.release 6583999a6e489be528f1729a
```

Note: private values come back masked on the first read after import, so they show as empty until the next apply writes the configured values.
