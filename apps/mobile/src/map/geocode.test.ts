import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  boundsForHits,
  buildAutocompleteParams,
  buildNearbyParams,
  buildNominatimSearchParams,
  classifySearchQuery,
  ensureMinSearchBox,
  expandViewBox,
  filterHitsInViewBox,
  matchCategoryPrefix,
  matchExactCategory,
  parseStreetAddressQuery,
  pointInViewBox,
  searchQueryVariants,
  sortHitsNearToFar,
} from "./geocode.ts";

describe("buildNominatimSearchParams", () => {
  it("sets viewbox as west,north,east,south when bounded", () => {
    const params = buildNominatimSearchParams("Burger King", {
      viewbox: [-3.8, 40.4, -3.6, 40.5],
      bounded: true,
      limit: 8,
    });
    assert.equal(params.get("q"), "Burger King");
    assert.equal(params.get("viewbox"), "-3.8,40.5,-3.6,40.4");
    assert.equal(params.get("bounded"), "1");
    assert.equal(params.get("limit"), "8");
  });

  it("omits bounded when not requested", () => {
    const params = buildNominatimSearchParams("plaza", {
      viewbox: [0, 0, 1, 1],
      bounded: false,
    });
    assert.equal(params.get("viewbox"), "0,1,1,0");
    assert.equal(params.get("bounded"), null);
  });

  it("uses street= for structured address search", () => {
    const params = buildNominatimSearchParams("", {
      street: "18 Avenida Las Golondrinas",
      acceptLanguage: "es",
    });
    assert.equal(params.get("street"), "18 Avenida Las Golondrinas");
    assert.equal(params.get("q"), null);
    assert.equal(params.get("countrycodes"), "es");
    assert.equal(params.get("accept-language"), "es");
  });
});

describe("buildAutocompleteParams", () => {
  it("biases with viewbox without hard bound", () => {
    const params = buildAutocompleteParams("burg", {
      viewbox: [-6, 37, -5.9, 37.4],
      acceptLanguage: "en",
      limit: 8,
    });
    assert.equal(params.get("q"), "burg");
    assert.equal(params.get("viewbox"), "-6,37,-5.9,37.4");
    assert.equal(params.get("bounded"), "0");
    assert.equal(params.get("accept-language"), "en");
    assert.equal(params.get("limit"), "8");
  });
});

describe("buildNearbyParams", () => {
  it("sets lat lon tag radius", () => {
    const params = buildNearbyParams(37.39, -5.98, "shop:hairdresser", {
      radius: 1200,
      limit: 20,
    });
    assert.equal(params.get("lat"), "37.39");
    assert.equal(params.get("lon"), "-5.98");
    assert.equal(params.get("tag"), "shop:hairdresser");
    assert.equal(params.get("radius"), "1200");
  });
});

describe("parseStreetAddressQuery", () => {
  it("parses trailing Spanish house number", () => {
    const p = parseStreetAddressQuery("Avenida Las Golondrinas, 18");
    assert.ok(p);
    assert.equal(p!.houseNumber, "18");
    assert.match(p!.street, /Las Golondrinas/i);
    assert.equal(p!.freeForm, "18 Avenida Las Golondrinas");
  });

  it("parses leading house number", () => {
    const p = parseStreetAddressQuery("18 Calle Enramadilla");
    assert.ok(p);
    assert.equal(p!.houseNumber, "18");
    assert.equal(p!.structuredStreet, "18 Calle Enramadilla");
  });

  it("returns null for brand names", () => {
    assert.equal(parseStreetAddressQuery("Burger King"), null);
  });
});

describe("classifySearchQuery", () => {
  it("routes street+number to address", () => {
    assert.equal(
      classifySearchQuery("Avenida Las Golondrinas, 18", "es").kind,
      "address",
    );
  });

  it("routes exact category in es and en to same tags", () => {
    const es = classifySearchQuery("peluquería", "es");
    const en = classifySearchQuery("hairdresser", "en");
    assert.equal(es.kind, "category");
    assert.equal(en.kind, "category");
    assert.deepEqual(es.osmTags, ["shop:hairdresser"]);
    assert.deepEqual(en.osmTags, ["shop:hairdresser"]);
  });

  it("routes named salon to name, not category", () => {
    assert.equal(classifySearchQuery("Peluquería Ana", "es").kind, "name");
  });

  it("routes brands to name", () => {
    assert.equal(classifySearchQuery("Burger King", "es").kind, "name");
  });
});

describe("matchCategoryPrefix", () => {
  it("suggests peluquerías while typing pelu", () => {
    const hint = matchCategoryPrefix("pelu", "es");
    assert.ok(hint);
    assert.deepEqual(hint!.osmTags, ["shop:hairdresser"]);
  });

  it("skips named places", () => {
    assert.equal(matchCategoryPrefix("Peluquería Ana", "es"), null);
  });
});

describe("matchExactCategory", () => {
  it("matches hair salon compact key", () => {
    const hint = matchExactCategory("hair salon", "en");
    assert.ok(hint);
    assert.deepEqual(hint!.osmTags, ["shop:hairdresser"]);
  });
});

describe("searchQueryVariants", () => {
  it("expands mc donalds into collapsed and McDonald's", () => {
    const v = searchQueryVariants("mc donalds");
    assert.ok(v.includes("mc donalds"));
    assert.ok(v.includes("mcdonalds"));
    assert.ok(v.some((x) => /mcdonald/i.test(x)));
  });

  it("keeps a normal street query intact", () => {
    const v = searchQueryVariants("Calle Enramadilla");
    assert.equal(v[0], "Calle Enramadilla");
    assert.ok(v.includes("CalleEnramadilla"));
  });
});

describe("expandViewBox / filterHitsInViewBox", () => {
  const seville: [number, number, number, number] = [-6.05, 37.35, -5.9, 37.45];

  it("keeps a Seville pin and drops a Mongolia pin", () => {
    const hits = [
      { id: "1", label: "BK Sevilla", lon: -5.98, lat: 37.39 },
      { id: "2", label: "BK Mongolia", lon: 106.9, lat: 47.9 },
    ];
    const local = filterHitsInViewBox(hits, expandViewBox(seville, 2));
    assert.equal(local.length, 1);
    assert.equal(local[0]!.id, "1");
  });

  it("pointInViewBox respects edges", () => {
    assert.equal(pointInViewBox(-5.98, 37.39, seville), true);
    assert.equal(pointInViewBox(106.9, 47.9, seville), false);
  });

  it("boundsForHits pads a single hit", () => {
    const b = boundsForHits([{ id: "1", label: "x", lon: -6, lat: 37.4 }]);
    assert.ok(b);
    assert.ok(b![0]! < -6 && b![2]! > -6);
  });

  it("ensureMinSearchBox widens a tiny street viewport", () => {
    const tiny: [number, number, number, number] = [
      -5.981, 37.389, -5.979, 37.391,
    ];
    const wide = ensureMinSearchBox(tiny, 0.06);
    assert.ok(wide[2]! - wide[0]! >= 0.12 - 1e-9);
    assert.ok(wide[3]! - wide[1]! >= 0.12 - 1e-9);
  });

  it("sortHitsNearToFar orders by distance to center", () => {
    const sorted = sortHitsNearToFar(
      [
        { id: "far", label: "f", lon: -5.9, lat: 37.4 },
        { id: "near", label: "n", lon: -5.98, lat: 37.39 },
      ],
      -5.98,
      37.39,
    );
    assert.equal(sorted[0]!.id, "near");
    assert.equal(sorted[1]!.id, "far");
  });
});
