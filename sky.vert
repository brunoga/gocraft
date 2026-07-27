#version 330 core

in vec2 pos;

out vec2 vNdc;

void main() {
    vNdc = pos;
    gl_Position = vec4(pos, 0.0, 1.0);
}
