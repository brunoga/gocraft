# GoCraft

A Minecraft like game written in go, just for fun!

![ScreenShot](https://i.imgur.com/vrGRDg1.png)

## Features

- Basic terrain generation
- Add and Remove blocks.
- Move and fly.
- Multiplayer support
- Sky lighting: caves and enclosed spaces get dark, and overhangs cast soft shadows. Lighting updates as you dig and build.
- Day-night cycle: the sun arcs across the sky, shifting the shading angle and sky colour, and the world darkens at night (caves stay black). Set the cycle length with `-daylen <seconds>` (default 600; e.g. `gocraft -daylen 60` for a fast cycle).
- Rendered sky: a gradient sky (horizon to zenith, with a warm glow on the sun's side at dawn and dusk) plus sun and moon discs that track across the sky as time passes.
- Torches: place a torch (the first item in the hotbar) to light up caves and dark rooms. A torch placed on a floor stands upright; placed against a wall it mounts diagonally, leaning out from the surface. It's a little three-part model — a spotted wooden handle, a band of ash, and a glowing ember tip — and its light spreads, dims with distance, and glows even at night. Break the block a torch is attached to and the torch comes off with it.
- Fire: place fire (the second item in the hotbar) on or next to wood, leaves, or plants and watch it spread to nearby fuel, cast its own flickering block light, and burn itself out to air once the fuel is gone. Fire on non-flammable surroundings simply dies out.
- Smoke: fires and torches give off rising smoke, dimming at night with the rest of the world. Two implementations are included and can be switched live with **G** to compare them: a **particle** system (soft GPU puffs that drift, grow, and fade as they climb) and a **volumetric** one (a cellular density grid where smoke rises, spreads, and dissipates as a continuous cloud). The current method is shown in the window title.

## Dependencies

### For go

- go 1.21+

### For glfw

- On macOS, you need Xcode or Command Line Tools for Xcode (`xcode-select --install`) for required headers and libraries.
- On Ubuntu/Debian-like Linux distributions, you need `libgl1-mesa-dev` and `xorg-dev` packages.
- On CentOS/Fedora-like Linux distributions, you need `libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel mesa-libGL-devel libXi-devel` packages.


## Install

`go get github.com/icexin/gocraft`

## Run

Suppose `$GOPATH/bin` is in your `PATH` env, use command below to run. 

`cd $GOPATH/src/github.com/icexin/gocraft && gocraft`

## How to play

- W, S, A, D to move around.
- Hold Left Shift while moving forward to run.
- TAB to toggle flying mode.
- SPACE to jump.
- Left and right click to add/remove block.
- E,R to cycle through the blocks (the torch and fire are the first two items in the hotbar).
- O to toggle ambient occlusion.
- G to switch the smoke rendering method (particles / volumetric).
- Alt+F to toggle fullscreen.

By default fullscreen uses the current desktop resolution. Pass `-fs WxH`
(e.g. `gocraft -fs 1920x1080`) to force a specific fullscreen resolution.

## Multiplayer

Multiplayer is supported now!

The server code is at https://github.com/icexin/gocraft-server .

You can use `gocraft -s gocraft.icexin.com` to connect the public server.

Since the player on public server is anonymous, be carefull for your work!

If any network error occurs, the game will end with a panic, may changed in the future.

Local cache is saved as `cache_$server.db`, you can use `gocraft -db xxx.db` to offline use.

## Roadmap

- [x] Persistent changed blocks
- [x] Multiplayer support
- [x] Ambient Occlusion support

## Implementation Details

Many implementations is inspired by https://github.com/fogleman/Craft, thanks for Fogleman's good work!

Multiplayer is implementated used a duplex rpc call, client can call server to update blocks or fetch chunks, server can also push changes to clients. 
