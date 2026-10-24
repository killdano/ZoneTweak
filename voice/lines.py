# Contraband radio voice lines: id -> text
nums = ["one","two","three","four","five","six","seven","eight","nine","ten","eleven","twelve","thirteen","fourteen","fifteen"]
L = {}
for i, n in enumerate(nums[:12]):
    c = i + 1
    L[f"c_walker_{c}"] = f"{n.capitalize()} walker," if c == 1 else f"{n.capitalize()} walkers,"
    L[f"c_heavy_{c}"] = f"{n.capitalize()} heavy," if c == 1 else f"{n.capitalize()} heavies,"
    L[f"c_leech_{c}"] = f"{n.capitalize()} leech drone," if c == 1 else f"{n.capitalize()} leech drones,"
L["c_walker_many"] = "A whole pack of walkers,"
L["c_heavy_many"] = "A lot of heavies,"
L["c_leech_many"] = "A swarm of leech drones,"
for d in ["north", "east", "south", "west"]:
    L[f"d_{d}"] = f"{d} door."
for i, n in enumerate(nums):
    L[f"w_{i+1}"] = f"Wave {n}, inbound."
L["w_next"] = "Next wave, inbound."
L["out_1"] = "That's all I've got."
L["out_2"] = "Hold the line."
L["out_3"] = "Watch your back out there."
# callouts from the radio speaker
L["x_start"] = "Here they come! Get that device built. Lights are out."
L["x_genblown"] = "You lost a generator! Get it fixed! The work order card on its panel says what it needs, one step at a time."
L["x_leech"] = "Leech on a generator! Rip its tether off, or zap the leech with the prod. The prod kills leeches for good, as many as you like. It'll drop a robot too, but the third robot breaks it."
L["x_stolen"] = "They've got the device! Get it back!"
L["x_exposed"] = "The device is outside your line!"
L["x_armed"] = "Harpoon's loaded."
L["x_recovered"] = "Got it back. Nice."
L["x_pickup"] = "Pick up the radio. I've got intel."
L["x_won"] = "That's it! You're out! Go, go, go!"
L["x_lost"] = "They took it. It's over."
L["x_livewire"] = "Live wire. Let them walk into it."
# build 2: the screwdriver hunt, and the wardens getting serious
L["h_need"] = "You need a screwdriver for those screws. There's one somewhere in your room."
L["h_floor_north"] = "Try the floor, over by the north wall."
L["h_floor_east"] = "Try the floor, over by the east wall."
L["h_floor_south"] = "Try the floor, over by the south wall."
L["h_floor_west"] = "Try the floor, over by the west wall."
L["h_gen"] = "Check your generators. One of the panels has a tool clip on it."
L["h_radio"] = "Look down by the radio."
L["h_console"] = "Look under the console."
L["x_genlow"] = "Generator's running low! Pump it!"
L["x_ringdown"] = "Your ring is down! They're coming for the device!"
