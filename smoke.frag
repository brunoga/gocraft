#version 330 core

in float vAlpha;

uniform float daylight;  // 0..1, darkens smoke at night like the terrain

out vec4 FragColor;

void main() {
    // Soft round puff: radial falloff from the point-sprite centre.
    vec2 d = gl_PointCoord - vec2(0.5);
    float r = length(d);
    float mask = smoothstep(0.5, 0.15, r);
    if (mask <= 0.0) {
        discard;
    }
    // Neutral grey smoke, dimmed at night so it doesn't glow in the dark.
    float shade = 0.55 * (0.25 + 0.75 * daylight);
    FragColor = vec4(vec3(shade), mask * vAlpha);
}
