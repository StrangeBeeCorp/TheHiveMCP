# The untrusted-data boundary

This page holds the reasoning behind the untrusted-data wrapping of TheHive field values returned to the model.

## Untrusted-data wrapping (deny-by-default)

TheHive field values returned to the LLM are wrapped in `[UNTRUSTED_DATA]...[/UNTRUSTED_DATA]` boundary tags so the model can tell data from instructions. The
policy is **deny-by-default**: every string value is wrapped _unless_ its field name is on a small trusted allowlist of structural identifiers, enums/control
values, and dates (`_id`, `_type`, `status`, `dataType`, date fields, and more). This means `customFields` values, attachment names, and any field added to the
TheHive SDK in the future are wrapped automatically — there is no longer an allowlist of "untrusted" fields that has to be kept in sync. Embedded boundary
markers are escaped so the boundary cannot be broken out of. The one whole-entity exception is the MITRE ATT&CK catalog (`entity-type: pattern`): it is
reference data imported by an administrator, not case material, so a technique's name, description, URL and platform list are returned unfenced. Boundary
markers inside them are still escaped. A procedure citing a technique is _not_ exempt — only its catalog references (`patternId`, `patternName`, `tactic`) are
trusted; its analyst-written description stays wrapped.
