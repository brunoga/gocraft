#version 330 core

in vec2 Tex;
in float AO;
in float Light;
in float Blocklight;
uniform sampler2D tex;

out vec4 FragColor;

void main() {
    vec3 color = vec3(texture(tex, vec2(Tex.x, 1-Tex.y)));
    if (color == vec3(1,0,1)) {
        discard;
    }
    // AO/Light/Blocklight are 1/1/0 for the avatar; referencing them keeps the
    // shared vertex attributes active so the stride matches the block geometry.
    FragColor = vec4(color * AO * max(Light, Blocklight), 1);
}
