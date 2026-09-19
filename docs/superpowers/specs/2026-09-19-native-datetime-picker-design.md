# Announce copy + native datetime pickers — design

Date: 2026-09-19  
Status: approved  
Scope: `apps/mobile` announce / offer / edit spot datetime UX

## Copy

- `announce.submit`: "Publicar" / "Publish" (no "7 days")
- `map.alert.announced.message`: "Plaza publicada." / "Spot published." (no duration)
- Backend `listed_until` unchanged; not presented as a product promise on publish

## Datetime UX

Replace free-text ISO inputs with `@react-native-community/datetimepicker` in:

- Announce modal — preferred departure
- Account spot edit — preferred departure
- Spot sheet offer — exchange datetime

Pattern: pressable showing localized formatted datetime; opens system date+time picker. Shared small component under `apps/mobile/src/ui/`.

## Out of scope

Changing listing duration policy or API fields.
