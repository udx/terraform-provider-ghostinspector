# ghostinspector_suite

Manages a Ghost Inspector suite and its default test settings. Creation adopts an existing suite with the same `name` in the folder instead of creating a duplicate. Suite variables are managed separately by `ghostinspector_suite_variables`.

## Example

```hcl
resource "ghostinspector_suite" "release" {
  name        = "Automated Tests"
  folder_id   = ghostinspector_folder.app.id
  description = "Release gate suite"

  browser   = "chrome"
  region    = "us-east-1"
  user_agent = "udx_custom_ua"

  schedule = {
    enabled  = true
    interval = "daily"
    time     = "08:00"
  }
}
```

## Argument reference

- `name` (string, required) - Suite name. Used for adopt-by-name matching on create.
- `suite_id` (string) - Existing suite ID to adopt on create. When set and the suite exists, it is adopted instead of created; when set and missing, creation fails rather than creating a different suite. Changing it replaces the resource.
- `folder_id` (string) - Owning folder. Folderless suites are supported.
- `description` (string) - Suite description. Write-only: the API discards it on update and never returns it on read, so the configured value is kept in state.
- `schedule` (object) - `enabled`, `interval` (for example `daily`), `time` (for example `08:00`). Write-only for the same reason.
- Settings attributes (`browser`, `region`, `user_agent`, `geolocation`, `max_wait_delay`, `max_ajax_delay`, `global_step_delay`, `final_delay`, `auto_retry`, `screenshot_compare_enabled`, `screenshot_compare_threshold`, `fail_on_javascript_error`) - null leaves the API-side value unmanaged.

All settings are pushed as the suite's default test settings, inherited by tests that do not override them.

## Attribute reference

- `id` - Ghost Inspector suite ID.

## Deletion safety

Destroying a suite is refused while it still contains tests (deleting a suite deletes its tests). Destroy the `ghostinspector_test` resources first.

## Import

```bash
tofu import ghostinspector_suite.release 6583999a6e489be528f1729a
```
