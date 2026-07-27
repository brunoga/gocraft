#version 330 core

in vec2 Tex;
in float diff;
in float fog_factor;
in float AO;
in float Light;
in float Blocklight;
in vec3 Vdir;
uniform sampler2D tex;
uniform float aoflag;
uniform float daylight;
uniform vec3 sundir;

out vec4 FragColor;

void main() {
    vec3 color = vec3(texture(tex, vec2(Tex.x, 1-Tex.y)));
    if (color == vec3(1,0,1)) {
        discard;
    }
    float df = diff;
    if (color == vec3(1,1,1)) {
        df = 1- diff * 0.2;
    }
    vec3 ambient = 0.5 * vec3(1, 1, 1);
    vec3 diffcolor = df * 0.5 * vec3(1,1,1);
    color = (ambient + diffcolor) * color;
    // Ambient occlusion: darken vertices whose neighbouring blocks occlude
    // ambient light. AO is 1 (fully lit) .. 0 (fully occluded). aoflag toggles
    // the effect at runtime (0 = off -> no darkening).
    float ao_factor = mix(1.0, mix(0.35, 1.0, AO), aoflag);
    color *= ao_factor;
    // Illumination is the brighter of skylight (scaled by the day-night daylight
    // level) and block light (torches, unaffected by time of day). A sealed cave
    // is black unless a torch lights it; a torch-lit spot glows even at night.
    float lit = max(Light * daylight, Blocklight);
    color *= lit;
    // Fog fades distant terrain into the sky gradient in the view direction, so
    // the horizon matches the sky exactly (same skyBackground, minus the discs).
    vec3 fogcolor = skyBackground(normalize(Vdir), sundir, daylight);
    color = mix(color, fogcolor, fog_factor);
    FragColor = vec4(color, 1);
}
