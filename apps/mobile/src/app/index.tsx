import { Camera, Map } from "@maplibre/maplibre-react-native";
import { StyleSheet, View } from "react-native";

import { barcelonaCenter, mapStyleUrl } from "@/config";

export default function MapScreen() {
  return (
    <View style={styles.fill}>
      <Map style={styles.fill} mapStyle={mapStyleUrl}>
        {/* Demotiles is a low-detail world style; zoom 2 shows continents.
            Phase 10 switches to a street style and a city zoom. */}
        <Camera initialViewState={{ center: barcelonaCenter, zoom: 2 }} />
      </Map>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
});
