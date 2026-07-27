#version 330 core

in vec2 Tex;
in float AO;
uniform sampler2D tex;

out vec4 FragColor;

void main() {
    vec3 color = vec3(texture(tex, vec2(Tex.x, 1-Tex.y)));
    if (color == vec3(1,0,1)) {
        discard;
    }
    // AO is always 1 for the avatar; referencing it keeps the shared vertex
    // attribute active so the vertex stride matches the block geometry.
    FragColor = vec4(color * AO, 1);
}
