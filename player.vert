#version 330 core

in vec3 pos;
in vec2 tex;
in vec3 normal;
in float ao;

uniform mat4 matrix;

out vec2 Tex;
out float AO;

void main() {
    gl_Position = matrix *  vec4(pos, 1.0);
    Tex = tex;
    // The avatar reuses the block geometry, which carries a per-vertex ao
    // attribute (always 1 here). Pass it through so the attribute stays active.
    AO = ao;
}
