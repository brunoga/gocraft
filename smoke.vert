#version 330 core

in vec3 pos;
in float size;   // billboard diameter in world units
in float alpha;  // per-particle opacity

uniform mat4 matrix;  // view-projection
uniform float scale;  // viewport_height / (2 * tan(fov/2)), for perspective sizing

out float vAlpha;

void main() {
    gl_Position = matrix * vec4(pos, 1.0);
    // Perspective point size: world size projected to pixels at this depth.
    float px = size * scale / max(gl_Position.w, 0.0001);
    gl_PointSize = clamp(px, 1.0, 64.0);
    vAlpha = alpha;
}
