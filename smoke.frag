#version 330 core

in float vAlpha;

uniform float daylight;  // 0..1, darkens smoke at night like the terrain

out vec4 FragColor;

void main() {
    // Soft round puff: a feathered radial falloff from the point-sprite centre,
    // fading all the way to the middle so overlapping puffs read as a soft cloud
    // rather than stacked discs.
    vec2 d = gl_PointCoord - vec2(0.5);
    float r = length(d);
    float mask = smoothstep(0.5, 0.0, r);
    mask *= mask; // steeper centre weighting -> wispier edges
    if (mask <= 0.001) {
        discard;
    }
    // Neutral grey smoke, dimmed at night so it doesn't glow in the dark.
    float shade = 0.5 * (0.2 + 0.8 * daylight);
    FragColor = vec4(vec3(shade), mask * vAlpha);
}
