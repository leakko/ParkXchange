import { Text } from "react-native";

import { accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";
import { useStreetAddress } from "@/map/useStreetAddress";

type Props = {
  lon: number | null | undefined;
  lat: number | null | undefined;
  hint?: string | null | undefined;
};

/** Street address line for account lists (never raw lat/lon). */
export function StreetAddressMeta({ lon, lat, hint }: Props) {
  const { t } = useTranslation();
  const label = useStreetAddress(lon, lat, hint);
  return (
    <Text style={accountStyles.rowMetaTight} numberOfLines={2}>
      {label?.trim() || t("account.spots.locationUnknown")}
    </Text>
  );
}
