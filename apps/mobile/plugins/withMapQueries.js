const { withAndroidManifest } = require("expo/config-plugins");

const VIEW = "android.intent.action.VIEW";

const schemes = [
  { scheme: "google.navigation" },
  { scheme: "waze" },
  { scheme: "maps" },
  { scheme: "geo" },
  { scheme: "https", host: "www.google.com" },
  { scheme: "https", host: "www.waze.com" },
];

function intentFilter(scheme, host) {
  const data = host
    ? { $: { "android:scheme": scheme, "android:host": host } }
    : { $: { "android:scheme": scheme } };
  return {
    action: [{ $: { "android:name": VIEW } }],
    data: [data],
  };
}

module.exports = function withMapQueries(config) {
  return withAndroidManifest(config, (mod) => {
    const manifest = mod.modResults.manifest;
    if (!manifest.queries) {
      manifest.queries = [{}];
    }
    const queries = manifest.queries[0];
    queries.intent = queries.intent || [];

    for (const { scheme, host } of schemes) {
      const already = queries.intent.some((entry) => {
        const data = entry.data?.[0]?.$ || {};
        return data["android:scheme"] === scheme && (data["android:host"] || undefined) === host;
      });
      if (!already) {
        queries.intent.push(intentFilter(scheme, host));
      }
    }

    return mod;
  });
};
