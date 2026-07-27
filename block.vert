#version 330 core

in vec3 pos;
in vec2 tex;
in vec3 normal;
in float ao;
in float light;
in float blocklight;

uniform mat4 matrix;
uniform vec3 camera;
uniform float fogdis;
uniform vec3 sundir;

out vec2 Tex;
out float diff;
out float fog_factor;
out float AO;
out float Light;
out float Blocklight;
out vec3 Vdir;

void main() {
    gl_Position = matrix *  vec4(pos, 1.0);

    float camera_distance = distance(pos, camera);
    fog_factor = pow(clamp(camera_distance/fogdis, 0, 1), 4);
    Tex = tex;
    // Directional sunlight from the (time-varying) sun direction.
    diff = max(0, dot(normal, sundir));
    AO = ao;
    Light = light;
    Blocklight = blocklight;
    // View ray toward this vertex, used to fade fog into the sky gradient.
    Vdir = pos - camera;
}
