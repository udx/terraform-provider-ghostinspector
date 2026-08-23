# ghostinspector_folder

Manages a Ghost Inspector folder. Creation adopts an existing folder with the same `name` instead of creating a duplicate.

The Ghost Inspector API has no folder deletion endpoint: destroying this resource only removes it from state and leaves the folder in place (a warning is emitted). Delete folders in the Ghost Inspector UI when no longer needed.

## Example

```hcl
resource "ghostinspector_folder" "app" {
  name = "My App"
}
```

## Argument reference

- `name` (string, required) - Folder name. Renamed in place.
- `organization` (string) - Owning organization ID. When null, creation tries the organizations visible to the API key until one accepts the write. Keys scoped to one organization should set this explicitly.

## Attribute reference

- `id` - Folder ID.
- `organization` - Owning organization ID.

## Import

```bash
tofu import ghostinspector_folder.app 5e8fb74436cfd03c50746853
```
