#version 330 core

in vec3 pos;
in vec2 tex;
in vec3 normal;
in float ao;
in float light;
in float blocklight;

uniform mat4 matrix;

out vec2 Tex;
out float AO;
out float Light;
out float Blocklight;

void main() {
    gl_Position = matrix *  vec4(pos, 1.0);
    Tex = tex;
    // The avatar reuses the block geometry, which carries per-vertex ao, light
    // and blocklight attributes. Pass them through so they stay active.
    AO = ao;
    Light = light;
    Blocklight = blocklight;
}
