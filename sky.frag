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

    // Background gradient + horizon glow (shared with terrain fog).
    vec3 col = skyBackground(rd, sundir, daylight);

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
