// Shared sky-colour function, injected after the #version line into both the
// sky shader and the block fragment shader (see withSkyCommon). It returns the
// background sky colour along a view ray -- the vertical gradient plus the warm
// dawn/dusk glow on the sun's side of the horizon -- but NOT the sun or moon
// discs. The sky shader adds those on top; terrain fog fades to this so the
// horizon has no seam.
vec3 skyBackground(vec3 rd, vec3 sundir, float daylight) {
    float el = rd.y; // view-ray elevation, -1 (down) .. 1 (up)

    vec3 dayHorizon   = vec3(0.57, 0.71, 0.77);
    vec3 dayZenith    = vec3(0.24, 0.46, 0.78);
    vec3 nightHorizon = vec3(0.02, 0.03, 0.08);
    vec3 nightZenith  = vec3(0.01, 0.01, 0.04);
    vec3 horizonC = mix(nightHorizon, dayHorizon, daylight);
    vec3 zenithC  = mix(nightZenith, dayZenith, daylight);
    vec3 col = mix(horizonC, zenithC, pow(clamp(el, 0.0, 1.0), 0.45));

    // How low the sun is: 0 when high (day) or well below the horizon (deep
    // night), 1 right at the horizon (dawn/dusk).
    float lowSun = clamp(1.0 - abs(sundir.y) / 0.35, 0.0, 1.0);

    // Horizontal (azimuthal) twilight gradient: at dawn/dusk the sky is bright
    // on the sun's side and falls off to a darker, cooler blue on the opposite
    // side, blending smoothly across. No effect when the sun is high or deep
    // below the horizon.
    vec3 rdH = normalize(vec3(rd.x, 0.001, rd.z));
    vec3 sdH = normalize(vec3(sundir.x, 0.001, sundir.z));
    float toward = dot(rdH, sdH) * 0.5 + 0.5; // 0 opposite the sun .. 1 toward it
    float sideFall = mix(1.0, mix(0.28, 1.0, toward), lowSun);
    col *= sideFall;
    // Nudge the far (anti-sun) side cooler/bluer as it darkens.
    col = mix(col, col * vec3(0.7, 0.85, 1.15), lowSun * (1.0 - toward) * 0.5);

    // Warm glow concentrated on the sun's side of the horizon (east at sunrise,
    // west at sunset), strongest near the horizon.
    vec3 warm = vec3(1.0, 0.55, 0.22);
    float sunSide = max(dot(rdH, sdH), 0.0);
    float horizonBand = pow(1.0 - clamp(abs(el), 0.0, 1.0), 3.0);
    col += warm * horizonBand * pow(sunSide, 2.0) * lowSun * 0.9;

    return col;
}
