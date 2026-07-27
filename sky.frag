#version 330 core

in vec2 vNdc;

uniform mat4 invvp;     // inverse view-projection, to rebuild the view ray
uniform vec3 campos;    // camera world position
uniform vec3 sundir;    // direction to the sun (normalised)
uniform float daylight; // 0 night .. 1 day

out vec4 FragColor;

void main() {
    // Reconstruct the world-space view ray through this pixel.
    vec4 far = invvp * vec4(vNdc, 1.0, 1.0);
    vec3 rd = normalize(far.xyz / far.w - campos);

    float el = rd.y; // view-ray elevation, -1 (down) .. 1 (up)

    // Vertical gradient: horizon -> zenith, blended between night and day.
    // dayHorizon matches the terrain fog colour so the horizon seam is soft.
    vec3 dayHorizon   = vec3(0.57, 0.71, 0.77);
    vec3 dayZenith    = vec3(0.24, 0.46, 0.78);
    vec3 nightHorizon = vec3(0.02, 0.03, 0.08);
    vec3 nightZenith  = vec3(0.01, 0.01, 0.04);
    vec3 horizonC = mix(nightHorizon, dayHorizon, daylight);
    vec3 zenithC  = mix(nightZenith, dayZenith, daylight);
    vec3 col = mix(horizonC, zenithC, pow(clamp(el, 0.0, 1.0), 0.45));

    // Warm dawn/dusk glow on the sun's side of the horizon: east at sunrise,
    // west at sunset. Strong near the horizon, and only while the sun is low.
    vec3 warm = vec3(1.0, 0.55, 0.22);
    vec3 rdH = normalize(vec3(rd.x, 0.001, rd.z));
    vec3 sdH = normalize(vec3(sundir.x, 0.001, sundir.z));
    float sunSide = max(dot(rdH, sdH), 0.0);
    float horizonBand = pow(1.0 - clamp(abs(el), 0.0, 1.0), 3.0);
    float lowSun = clamp(1.0 - abs(sundir.y) / 0.35, 0.0, 1.0);
    col += warm * horizonBand * pow(sunSide, 2.0) * lowSun * 0.9;

    // Sun: a soft halo plus a bright disc, fading out as it sets.
    float sunUp = clamp(sundir.y * 4.0 + 0.3, 0.0, 1.0);
    float sunCos = dot(rd, sundir);
    col += vec3(1.0, 0.85, 0.6) * pow(max(sunCos, 0.0), 64.0) * sunUp * 0.35;
    float sunDisc = smoothstep(0.9975, 0.9990, sunCos);
    col = mix(col, vec3(1.0, 0.97, 0.88), sunDisc * sunUp);

    // Moon: opposite the sun, so it is up at night.
    vec3 moondir = -sundir;
    float moonUp = clamp(moondir.y * 4.0, 0.0, 1.0);
    float moonCos = dot(rd, moondir);
    col += vec3(0.5, 0.55, 0.7) * pow(max(moonCos, 0.0), 128.0) * moonUp * 0.2;
    float moonDisc = smoothstep(0.9982, 0.9992, moonCos);
    col = mix(col, vec3(0.92, 0.94, 1.0), moonDisc * moonUp);

    FragColor = vec4(col, 1.0);
}
