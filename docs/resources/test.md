# ghostinspector_test

Manages one Ghost Inspector test: metadata and steps.

Creation adopts an existing test with the same `name` in the suite instead of creating a duplicate. Metadata changes update in place. Step changes replace the test: the API cannot update steps in place, so the provider exports the current definition, overlays the managed fields and steps, deletes the test, and re-imports it. The replacement receives a new test ID; historical results stay with the removed test. If the replacement import fails, the exported definition is restored on a best-effort basis.

Other resources referencing this test's `id` (for example an `execute` step's value) are updated through the dependency graph when the ID changes.

## Example

```hcl
resource "ghostinspector_test" "login_module" {
  suite_id    = ghostinspector_suite.release.id
  name        = "Login User"
  import_only = true

  steps = [
    { command = "open", value = "/login" },
    { command = "assign", target = "input[name=email]", value = "{{defaultUser}}" },
    { command = "assign", target = "input[name=password]", value = "{{defaultUserPassword}}", private = true },
    { command = "click", target = ".btn-login" },
  ]
}

resource "ghostinspector_test" "checkout" {
  suite_id = ghostinspector_suite.release.id
  name     = "Checkout"

  steps = [
    { command = "execute", value = ghostinspector_test.login_module.id },
    { command = "open", value = "/checkout" },
  ]
}
```

## Argument reference

- `suite_id` (string, required) - Owning suite. Changing it replaces the test.
- `name` (string, required) - Test name. Used for adopt-by-name matching on create.
- `start_url` (string) - Start URL; may contain `{{variables}}`.
- `import_only` (bool) - Mark as an import-only shared module.
- `steps` (list of objects) - Managed steps. Null leaves steps unmanaged. Changing steps replaces the test.
- Settings attributes (`browser`, `region`, `user_agent`, `geolocation`, `max_wait_delay`, `max_ajax_delay`, `global_step_delay`, `final_delay`, `auto_retry`, `screenshot_compare_enabled`, `screenshot_compare_threshold`, `fail_on_javascript_error`) - null leaves the API-side value unmanaged (inherited from the suite).

### Step object

- `command` (string, required) - For example `open`, `click`, `assign`, `assertText`, `screenshot`, `execute`.
- `target` (string) - Single selector. Conflicts with `targets`.
- `targets` (list of strings) - Ordered selector candidates. Conflicts with `target`.
- `value` (string) - Step value (text, pause ms, module test ID for `execute`, ...).
- `condition` (string) - JSON condition object, for example `{"statement": "return true;"}`. Compared semantically.
- `variable_name` (string) - Variable name for commands that store a value.
- `notes` (string) - Free-form notes.
- `optional` (bool) - Allow failure without failing the test.
- `private` (bool) - Mask the step value in the UI and results.

## Attribute reference

- `id` - Ghost Inspector test ID. Changes when steps are replaced.

## Import

```bash
tofu import ghostinspector_test.checkout 6a8a199960825c6d59fa081e
```
