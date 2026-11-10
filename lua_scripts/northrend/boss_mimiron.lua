-- Mimiron (Ulduar, phase-machine controller) -- Lua port of the
-- engine-fired arms of boss_mimironAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 384-682).
-- Entry (ulduar.h): NPC_MIMIRON = 33350.
--
-- PORTED (engine-fired hooks only, C++-exact spell IDs / delays / order):
--   constructor (:391) react-passive leg, on spawn (event 5).
--   Reset (:456): DONE early-return gate + _Reset() SetBossState
--     NOT_STARTED (PORTED tick 259 via creature:SetBossState,
--     lua_creature_events.go:5917) + RemoveFlag NON_ATTACKABLE +
--     self-cast WELD, on reset (23). The DONE gate rides FIRST,
--     before cancelTimers: C++ returns before _Reset() when DONE,
--     so the outtro chain scheduled by JustDied survives a
--     death-path reset (the thorim finished-guard precedent).
--   JustEngagedWith (:424): SetBossState IN_PROGRESS PORTED tick 259
--     (BossAI::JustEngagedWith passthrough, base-first) + SetFlag
--     NON_ATTACKABLE + WELD removal, on enter combat (1). See
--     DEVIATION note below for the unmodeled vehicle gate.
--   JustDied (:442) + EVENT_OUTTRO_1/2/3 chain: SetBossState DONE
--     PORTED tick 259 (first per C++), CombatStop, gravity off,
--     SLEEP_VISUAL_1, DESPAWN_ASSAULT_BOTS (DoCastAOE = self-cast
--     convention), then 7s OUTTRO_1 (SLEEP_VISUAL_1 removal, SLEEP_VISUAL_2,
--     faction friendly), +3s OUTTRO_2 (Talk V07TRON_DEATH), +11s OUTTRO_3
--     (TELEPORT_VISUAL, NOT_SELECTABLE flag, DespawnOrUnsummon 1s).
--   JustDied ExitVehicle + MoveJump leg PORTED (tick 475): the C++ comment
--     says the jump exists because ExitVehicle's offset position is
--     unimplemented -- the intended final position IS the jump destination
--     (x + 10*cos(o), y + 10*sin(o), z, o), so the tick-466
--     ExitVehicle(x, y, z, o) form lands there directly (GetX/GetY/GetZ/
--     GetO resolve via the generic object fallback, scripting/object.go:185).
--   JustEngagedWith EVENT_INTRO_1 PORTED (tick 475): Talk, latch-gated
--     (SAY_HARDMODE_ON when _fireFighter, else SAY_MKII_ACTIVATE), 1500ms
--     after engage; EVENT_INTRO_2/INTRO_3 PORTED (tick 557): INTRO_2's
--     MKII legs (SEAT_7 cast, FREEZE_ANIM removal, flag removal) and
--     INTRO_3's DoAction to MKII (DO_HARDMODE_MKII when _fireFighter,
--     else DO_START_MKII) resolve MKII via the instance GUID
--     (DATA_LEVIATHAN_MK_II twin of GetVehicleBase/GetVehicleCreatureBase
--     -- same target creature, different lookup).
--   DoAction (:397) receiver PORTED (tick 475, event 38): all five cases.
--     DO_ACTIVATE_HARD_MODE sets the _fireFighter latch (per-GUID state)
--     + DoZoneInCombat; DO_ACTIVATE_VX001 / DO_ACTIVATE_AERIAL /
--     DO_ACTIVATE_V0L7R0N_2 schedule their activation chains (1s/5s/1s)
--     with the bridgeable legs ported per step (Talk legs, VX-001 summon
--     + FREEZE_ANIM, ACU summon, SEAT_1/2 casts and DoAction relays via
--     the Lua-kept summon refs, the DATA_VX_001 / DATA_AERIAL_COMMAND_UNIT
--     instance-GUID twins); DO_ACTIVATE_V0L7R0N_1 is Talk + the MKII
--     MoveTo P4_POS_1 leg (PORTED this tick, MoveTo twin). The receiver
--     is live for DO_ACTIVATE_VX001 (MKII MovementInform :807, tick 561),
--     DO_ACTIVATE_AERIAL (VX-001 DamageTaken :960, tick 561) and
--     DO_ACTIVATE_V0L7R0N_1 (ACU MovementInform :1229, tick 561) and
--     DO_ACTIVATE_V0L7R0N_2 (MKII MovementInform :817, this tick); the
--     computer self-destruct :1453 sender is live via
--     npc_mimiron_computer.lua (this tick).
--   EVENT_OUTTRO_2 cache summon PORTED (tick 475): the GO entry rides the
--     _fireFighter latch (now modeled) + IsHeroic RAID_MODE twin
--     (ulduar.h:299-302; 420-day respawn = 36288000s); the computer
--     DoAction(DO_DEACTIVATE_COMPUTER) leg is live via
--     npc_mimiron_computer.lua (this tick).
--
-- PORTED (tick 190, RegisterAuraEvent):
--   spell_mimiron_summon_flames_spread (64562) HandleTick — PreventDefaultAction;
--   if the caster has SPELL_FLAMES_PERIODIC_TRIGGER (64561) it casts
--   SPELL_SUMMON_FLAMES_SPREAD (64564) triggered on the aura target
--   (boss_mimiron.cpp:2615-2627). The EVENT_SUMMON_FLAMES boss-AI arm
--   stays out (see below).
--
-- PORTED (tick 193, RegisterAuraEvent):
--   spell_mimiron_weld (63339) HandleTick — the aura target (the
--   player riding in Mimiron's vehicle) casts a random RepairSpell
--   on the vehicle base every fifth tick and faces it every tick
--   (boss_mimiron.cpp:2760-2772); the passenger GetVehicleBase twin
--   is target:IsOnVehicle():GetOwner() (see below).
-- DOCUMENTED-ONLY (no behavior fabricated):
--   Activation-chain legs with no bridge: vehicle-base legs (SEAT_6/7/5/2
--     casts, SetFacingTo on the base -- GetVehicleBase /
--     GetVehicleCreatureBase have no binding on the handler surface),
--     MovePoint legs (no MovePoint binding), elevator/button GameObject
--     legs (no GO bridge), MKII legs in the activation chains
--     (vehicle-base SetFacingTo / MOUNT / MovePoint legs; the
--     instance-GUID MKII ref itself is live — INTRO_2/3 use it),
--     worldtrigger legs (EVENT_SUMMON_FLAMES, elevator
--     knockback), aerial EnterEvadeMode on Reset (no bridge; event 25 unused),
--     Reset SEAT_3 cast on MKII via instance GUID.
--   JustEngagedWith vehicle-base leg: DoCast SEAT_6 on the vehicle base --
--     out (no GetVehicleBase binding); button GO flag leg -- out (no GO
--     bridge); EVENT_SUMMON_FLAMES -- out (worldtrigger via instance GUID).
--     (INTRO_2/INTRO_3 are ported — see the INTRO_1 bullet — via the
--     instance-GUID MKII twin, not the vehicle base.)
--   Reset instance/GO/cross-creature legs: aerial EnterEvadeMode,
--     elevator/button GO legs, SEAT_3 cast on MKII via instance GUID -- all out.
--   Bomb/rocket/mine SpellHitTarget legs live in the npc files, not
--     here: npc_mimiron_bomb_bot.lua / npc_mimiron_rocket_visual.lua /
--     npc_mimiron_proximity_mine.lua register ev15 (the frost-bomb has
--     no ev15 leg — seed/cancel only, npc_mimiron_frost_bomb.lua).
--
-- DEVIATIONS (checked against the CURRENT runtime, not memory):
--   * JustEngagedWith's `if (!me->GetVehicleBase()) return` gate is not
--     modeled: no GetVehicleBase binding exists on the creature handler
--     surface (grep of lua_creature_events.go / lua_objects.go /
--     scripting/object.go). The self legs below fire unconditionally.
--     (The weld aura arm below does NOT use this gate: its target is a
--     passenger, and the passenger twin target:IsOnVehicle():GetOwner()
--     — the Vehicle object the player rides, whose owner is the base
--     unit (lua_player.go IsOnVehicle, lua_vehicle.go GetOwner) — is
--     live. The boss's own vehicle-base gate stays unmodeled.)
--   * RemoveAurasDueToSpell has no binding; creature:RemoveAura (bridged,
--     lua_creature_events.go:1504) is the single-spell removal twin and
--     is used for the WELD / SLEEP_VISUAL_1 removals.
--   * The OUTTRO chain runs ungated: C++ UpdateAI's CASTING gate is not
--     modeled -- mimiron casts nothing else during the outtro, so the
--     gate cannot change the chain. Same for the activation-chain cast
--     steps (SEAT_1/2, MOUNT_VX_001, HALF_HEAL): his only other casts in
--     those phases are the chain steps themselves.
--   * JustDied's MoveJump is folded into the ExitVehicle position (see
--     above): when unseated the Go exit is a no-op and the 10yd jump is
--     skipped, where C++ would still jump -- the vehicle machine that
--     seats him is unported, so the deviation rides with it.
--   * The VX-001 / ACU summon refs kept in per-GUID state twin the
--     DATA_VX_001 / DATA_AERIAL_COMMAND_UNIT instance GUIDs: in C++ the
--     GUIDs are set when the instance creates those creatures; here the
--     only creators are the activation chains themselves, so the refs
--     are exactly the creatures the later chain steps address.
--   * No berserk: SAY_BERSERK (14) has no scheduling event in this class.

local NPC_MIMIRON = 33350
local BOSS_MIMIRON = 10 -- ulduar.h:46

local SPELL_WELD = 63339
local SPELL_FLAMES_PERIODIC_TRIGGER = 64561 -- boss_mimiron.cpp:131
local SPELL_SUMMON_FLAMES_SPREAD_TRIGGER = 64562 -- boss_mimiron.cpp:132
local SPELL_SUMMON_FLAMES_SPREAD = 64564 -- boss_mimiron.cpp:134
local SPELL_SLEEP_VISUAL_1 = 64393
local SPELL_SLEEP_VISUAL_2 = 64394
local SPELL_DESPAWN_ASSAULT_BOTS = 64463
local SPELL_TELEPORT_VISUAL = 41232
local SPELL_SEAT_1 = 52391
local SPELL_SEAT_2 = 63313
local SPELL_SEAT_5 = 63316
local SPELL_SEAT_6 = 63344
local SPELL_SEAT_7 = 63345
local SPELL_FREEZE_ANIM = 16245
local SPELL_MOUNT_VX_001 = 64388
local SPELL_MOUNT_MKII = 64387 -- boss_mimiron.cpp:100
local SPELL_TORSO_DISABLED = 64120 -- boss_mimiron.cpp:101
local SPELL_HALF_HEAL = 64188

local NPC_VX_001 = 33651
local NPC_AERIAL_COMMAND_UNIT = 33670

local GO_CACHE_OF_INNOVATION = 194789
local GO_CACHE_OF_INNOVATION_HERO = 194956
local GO_CACHE_OF_INNOVATION_FIREFIGHTER = 194957
local GO_CACHE_OF_INNOVATION_FIREFIGHTER_HERO = 194958

local SAY_HARDMODE_ON = 1
local SAY_MKII_ACTIVATE = 2
local SAY_MKII_DEATH = 4
local SAY_VX001_ACTIVATE = 5
local SAY_VX001_DEATH = 7
local SAY_AERIAL_ACTIVATE = 8
local SAY_AERIAL_DEATH = 10
local SAY_V07TRON_ACTIVATE = 11

local DO_START_MKII = 0
local DO_HARDMODE_MKII = 1
local DO_ACTIVATE_VX001 = 2
local DO_START_VX001 = 3
local DO_HARDMODE_VX001 = 4
local DO_ACTIVATE_AERIAL = 5
local DO_START_AERIAL = 6
local DO_HARDMODE_AERIAL = 7
local DO_ACTIVATE_V0L7R0N_1 = 10
local DO_ACTIVATE_V0L7R0N_2 = 11
local DO_ASSEMBLED_COMBAT = 12
local DO_ACTIVATE_HARD_MODE = 13
local DO_DEACTIVATE_COMPUTER = 15

-- boss_mimiron.cpp:327-340 (VX001SummonPos / ACUSummonPos).
local VX001_SUMMON_POS = { 2744.431, 2569.385, 364.3968, 3.141593 }
local ACU_SUMMON_POS = { 2744.650, 2569.460, 380.0, 3.141593 }

-- boss_mimiron.cpp:319-323 Waypoints enum + :346/:349 VehicleRelocation
-- (MoveTo twin ids): WP_MKII_P4_POS_1 = 2, WP_MKII_P4_POS_4 = 5.
local WP_MKII_P4_POS_1 = 2
local WP_MKII_P4_POS_4 = 5
local MKII_P4_POS_1 = { 2765.945, 2571.095, 364.0636 }
local MKII_P4_POS_4 = { 2761.215, 2568.875, 364.0636 }

local SAY_V07TRON_DEATH = 13

-- UlduarData instance GUID key for Leviathan Mk II (ulduar.h /
-- instance_ulduar.cpp:817; the npc_mimiron_proximity_mine.lua ev15
-- precedent): the Lua twin of C++ me->GetVehicleBase() /
-- GetVehicleCreatureBase() for the INTRO_2/INTRO_3 MKII legs — the
-- vehicle-base resolution has no binding, but the target creature
-- is the same MKII whose GUID the instance latches here.
local DATA_LEVIATHAN_MK_II = 32
local DATA_COMPUTER = 35 -- ulduar.h UlduarData (DATA_AERIAL_COMMAND_UNIT = 34, counting up)

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local REACT_PASSIVE = 0
local FACTION_FRIENDLY = 35

-- GUID-keyed CreateLuaEvent timers (C++ JustDied schedules the OUTTRO
-- chain on the EventMap; the frost-bomb / proximity-mine precedent).
local timers = {}

-- Per-GUID controller state (C++ boss_mimironAI members): the _fireFighter
-- latch (set by DO_ACTIVATE_HARD_MODE, cleared by Reset, read by the
-- INTRO_1 Talk and the OUTTRO_2 cache summon) + the VX-001 / ACU
-- creatures summoned by the activation chains, the Lua twin of the
-- DATA_VX_001 / DATA_AERIAL_COMMAND_UNIT instance GUIDs (kept across
-- Reset like the C++ GUIDs; cleared when the controller despawns).
local bossState = {}

local function stateFor(guid)
    local st = bossState[guid]
    if not st then
        st = { fireFighter = false }
        bossState[guid] = st
    end
    return st
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- Forward declarations for the cyclic OUTTRO chain (tharon_ja/gal_darah
-- idiom: plain assignments below, never `local function` re-declares)
-- plus the INTRO_1 step, defined below the engage handler that schedules it.
local outtro1, outtro2, outtro3, intro1, intro2, intro3

-- C++ constructor (:391): SetReactState(REACT_PASSIVE), _fireFighter false.
local function mimironSpawn(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    stateFor(creature:GetGUIDLow())
end

-- C++ Reset (:456), creature-local legs in C++ order: DONE
-- early-return gate (:457-458), _Reset() SetBossState NOT_STARTED,
-- then RemoveFlag NON_ATTACKABLE, _fireFighter = false, then self-cast
-- WELD. _Reset clears the event machine, so Lua timers are cancelled
-- here too. The VX-001 / ACU refs are kept (C++ instance GUIDs persist
-- across Reset); the latch is what Reset clears.
local function mimironReset(event, creature)
    if creature:GetBossState(BOSS_MIMIRON) == 3 then
        return
    end
    creature:SetBossState(BOSS_MIMIRON, 0)
    cancelTimers(creature:GetGUIDLow())
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    -- C++ Reset (:471-472): when _fireFighter, the computer gets
    -- DoAction(DO_DEACTIVATE_COMPUTER) before the latch clears.
    if stateFor(creature:GetGUIDLow()).fireFighter then
        local computer = creature:GetInstanceCreature(DATA_COMPUTER)
        if computer then
            computer:DoAction(DO_DEACTIVATE_COMPUTER)
        end
    end
    stateFor(creature:GetGUIDLow()).fireFighter = false
    creature:CastSpell(creature, SPELL_WELD)
end

-- C++ JustEngagedWith (:424), creature-local legs in C++ order:
-- BossAI::JustEngagedWith SetBossState IN_PROGRESS (base-first),
-- SetFlag NON_ATTACKABLE, remove WELD. Vehicle-base cast, button GO leg
-- and SUMMON_FLAMES stay out (see header); EVENT_INTRO_1 is scheduled
-- at 1500ms (latch-gated Talk, see intro1).
local function mimironEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_MIMIRON, 1)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveAura(SPELL_WELD)
    local guid = creature:GetGUIDLow()
    schedule(guid, "intro1", 1500, function()
        intro1(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_INTRO_1: Talk, latch-gated (SAY_HARDMODE_ON when
-- _fireFighter, else SAY_MKII_ACTIVATE), then EVENT_INTRO_2 in 5s.
intro1 = function(creature, guid)
    local st = bossState[guid]
    creature:Talk(st and st.fireFighter and SAY_HARDMODE_ON or SAY_MKII_ACTIVATE)
    schedule(guid, "intro2", 5000, function()
        intro2(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_INTRO_2: on the vehicle base (MKII): cast SEAT_7,
-- remove FREEZE_ANIM, remove NON_ATTACKABLE | NOT_SELECTABLE; then
-- EVENT_INTRO_3 in 2s. The vehicle-base resolution has no binding; the
-- MKII is resolved via the instance GUID instead (DATA_LEVIATHAN_MK_II
-- twin, see above) — same target creature, different lookup.
intro2 = function(creature, guid)
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        creature:CastSpell(mkii, SPELL_SEAT_7)
        mkii:RemoveAura(SPELL_FREEZE_ANIM)
        mkii:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    end
    schedule(guid, "intro3", 2000, function()
        intro3(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_INTRO_3: the vehicle base (MKII) gets
-- DO_HARDMODE_MKII when _fireFighter, else DO_START_MKII. Same
-- instance-GUID twin as INTRO_2. This is the only sender of those
-- two actions, so the MKII ev38 arms go live here.
intro3 = function(creature, guid)
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        local st = bossState[guid]
        mkii:DoAction(st and st.fireFighter and DO_HARDMODE_MKII or DO_START_MKII)
    end
end

-- C++ UpdateAI EVENT_OUTTRO_3: TELEPORT_VISUAL, NOT_SELECTABLE flag,
-- DespawnOrUnsummon(1s). The controller despawns, so its per-GUID
-- state is dropped here.
outtro3 = function(creature, guid)
    creature:CastSpell(creature, SPELL_TELEPORT_VISUAL)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:DespawnOrUnsummon(1000)
    bossState[guid] = nil
end

-- C++ UpdateAI EVENT_OUTTRO_2: Talk SAY_V07TRON_DEATH, then OUTTRO_3
-- in 11s. The cache summon rides the _fireFighter latch + IsHeroic
-- RAID_MODE twin (ulduar.h:299-302; 420-day respawn = 36288000s); the
-- computer DoAction(DO_DEACTIVATE_COMPUTER) leg is live via
-- npc_mimiron_computer.lua (this tick).
outtro2 = function(creature, guid)
    creature:Talk(SAY_V07TRON_DEATH)
    local st = bossState[guid]
    local heroic = creature:IsHeroic()
    local cache
    if st and st.fireFighter then
        -- C++ EVENT_OUTTRO_2 (:651-652): when _fireFighter, the computer
        -- gets DoAction(DO_DEACTIVATE_COMPUTER) before the cache summon.
        local computer = creature:GetInstanceCreature(DATA_COMPUTER)
        if computer then
            computer:DoAction(DO_DEACTIVATE_COMPUTER)
        end
        cache = heroic and GO_CACHE_OF_INNOVATION_FIREFIGHTER_HERO or GO_CACHE_OF_INNOVATION_FIREFIGHTER
    else
        cache = heroic and GO_CACHE_OF_INNOVATION_HERO or GO_CACHE_OF_INNOVATION
    end
    creature:SummonGameObject(cache, 2744.040, 2569.352, 364.3135, 3.124123, 36288000)
    schedule(guid, "outtro3", 11000, function()
        outtro3(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_OUTTRO_1: remove SLEEP_VISUAL_1, cast
-- SLEEP_VISUAL_2, faction friendly, then OUTTRO_2 in 3s.
outtro1 = function(creature, guid)
    creature:RemoveAura(SPELL_SLEEP_VISUAL_1)
    creature:CastSpell(creature, SPELL_SLEEP_VISUAL_2)
    creature:SetFaction(FACTION_FRIENDLY)
    schedule(guid, "outtro2", 3000, function()
        outtro2(creature, guid)
    end)
end

-- C++ JustDied (:442), creature-local legs in C++ order: SetBossState
-- DONE first, events.Reset (Lua timers cancelled), CombatStop(true),
-- SetDisableGravity(false), self-cast SLEEP_VISUAL_1, self-cast
-- DESPAWN_ASSAULT_BOTS (DoCastAOE), ExitVehicle() + the compensating
-- MoveJump 10yd forward (the C++ comment: ExitVehicle's offset position
-- is unimplemented, so MoveJump makes up for it) -- the tick-466
-- ExitVehicle(x, y, z, o) form lands at the jump destination directly;
-- no-op when unseated (see DEVIATIONS) -- then OUTTRO_1 in 7s. The
-- _fireFighter latch is NOT cleared here (OUTTRO_2 reads it).
local function mimironDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    creature:SetBossState(BOSS_MIMIRON, 3)
    cancelTimers(guid)
    creature:CombatStop(true)
    creature:SetDisableGravity(false)
    creature:CastSpell(creature, SPELL_SLEEP_VISUAL_1)
    creature:CastSpell(creature, SPELL_DESPAWN_ASSAULT_BOTS)
    local o = creature:GetO()
    creature:ExitVehicle(creature:GetX() + 10 * math.cos(o), creature:GetY() + 10 * math.sin(o), creature:GetZ(), o)
    schedule(guid, "outtro1", 7000, function()
        outtro1(creature, guid)
    end)
end

-- C++ DoAction DO_ACTIVATE_VX001 (:397-400) schedules
-- EVENT_VX001_ACTIVATION_1 in 1s. Steps are defined last-to-first so
-- each schedules the next without forward declarations. Steps 1/2
-- (vehicle-base SetFacingTo / SEAT_6), step 4 (elevator GO + worldtrigger
-- knockback) and step 8 (SEAT_2 on the vehicle base) stay out (no
-- GetVehicleBase / GO / instance-GUID bridge); the Talk, summon and
-- DoAction legs are ported. The summoned VX-001 is kept in per-GUID
-- state, the Lua twin of the DATA_VX_001 instance GUID the later
-- steps read.
local function vx001Activation9(creature, guid)
    local st = bossState[guid]
    local vx001 = st and st.vx001
    if vx001 then
        vx001:DoAction(st.fireFighter and DO_HARDMODE_VX001 or DO_START_VX001)
    end
end

local function vx001Activation8(creature, guid)
    schedule(guid, "vx001act9", 3000, function()
        vx001Activation9(creature, guid)
    end)
end

local function vx001Activation7(creature, guid)
    creature:Talk(SAY_VX001_ACTIVATE)
    schedule(guid, "vx001act8", 4000, function()
        vx001Activation8(creature, guid)
    end)
end

local function vx001Activation6(creature, guid)
    local st = bossState[guid]
    local vx001 = st and st.vx001
    if vx001 then
        creature:CastSpell(vx001, SPELL_SEAT_1)
    end
    schedule(guid, "vx001act7", 3500, function()
        vx001Activation7(creature, guid)
    end)
end

local function vx001Activation5(creature, guid)
    local vx001 = creature:SummonCreature(NPC_VX_001, VX001_SUMMON_POS[1], VX001_SUMMON_POS[2], VX001_SUMMON_POS[3], VX001_SUMMON_POS[4], 5, 120000)
    if vx001 then
        vx001:CastSpell(vx001, SPELL_FREEZE_ANIM)
        stateFor(guid).vx001 = vx001
    end
    schedule(guid, "vx001act6", 19000, function()
        vx001Activation6(creature, guid)
    end)
end

local function vx001Activation4(creature, guid)
    schedule(guid, "vx001act5", 6000, function()
        vx001Activation5(creature, guid)
    end)
end

local function vx001Activation3(creature, guid)
    creature:Talk(SAY_MKII_DEATH)
    schedule(guid, "vx001act4", 5000, function()
        vx001Activation4(creature, guid)
    end)
end

local function vx001Activation2(creature, guid)
    schedule(guid, "vx001act3", 1000, function()
        vx001Activation3(creature, guid)
    end)
end

local function vx001Activation1(creature, guid)
    schedule(guid, "vx001act2", 1000, function()
        vx001Activation2(creature, guid)
    end)
end

-- C++ DoAction DO_ACTIVATE_AERIAL (:401-403) schedules
-- EVENT_AERIAL_ACTIVATION_1 in 5s. Step 1 (SEAT_5 on the vehicle base)
-- stays out; the Talk, summon and DoAction legs are ported. The
-- summoned ACU is kept in per-GUID state (DATA_AERIAL_COMMAND_UNIT twin).
local function aerialActivation6(creature, guid)
    local st = bossState[guid]
    local aerial = st and st.aerial
    if aerial then
        aerial:DoAction(st.fireFighter and DO_HARDMODE_AERIAL or DO_START_AERIAL)
    end
end

local function aerialActivation5(creature, guid)
    creature:Talk(SAY_AERIAL_ACTIVATE)
    schedule(guid, "aerialact6", 8000, function()
        aerialActivation6(creature, guid)
    end)
end

local function aerialActivation4(creature, guid)
    local st = bossState[guid]
    local aerial = st and st.aerial
    if aerial then
        creature:CastSpell(aerial, SPELL_SEAT_1)
    end
    schedule(guid, "aerialact5", 2000, function()
        aerialActivation5(creature, guid)
    end)
end

local function aerialActivation3(creature, guid)
    local aerial = creature:SummonCreature(NPC_AERIAL_COMMAND_UNIT, ACU_SUMMON_POS[1], ACU_SUMMON_POS[2], ACU_SUMMON_POS[3], ACU_SUMMON_POS[4], 7)
    if aerial then
        stateFor(guid).aerial = aerial
    end
    schedule(guid, "aerialact4", 5000, function()
        aerialActivation4(creature, guid)
    end)
end

local function aerialActivation2(creature, guid)
    creature:Talk(SAY_VX001_DEATH)
    schedule(guid, "aerialact3", 5000, function()
        aerialActivation3(creature, guid)
    end)
end

local function aerialActivation1(creature, guid)
    schedule(guid, "aerialact2", 2500, function()
        aerialActivation2(creature, guid)
    end)
end

-- C++ DoAction DO_ACTIVATE_V0L7R0N_2 (:413-415) schedules
-- EVENT_VOL7RON_ACTIVATION_1 in 1s. Step 1 (MKII SetFacingTo PI) PORTED
-- this tick (SetFacingTo bridged, instance-resolved MKII); step 2 PORTED
-- this tick -- vx001 drops TORSO_DISABLED via creature:RemoveAura, the
-- single-spell removal twin of RemoveAurasDueToSpell (bridged,
-- lua_creature_events.go:1504; WELD/SLEEP precedent in this file), then
-- casts SPELL_MOUNT_MKII 64387 on MKII (cross-creature CastSpell
-- precedent: vol7ronActivation4); step 3 (MKII MovePoint P4_POS_4)
-- PORTED this tick via the MoveTo twin; steps 4 (aerial on the stored
-- VX-001 ref) and 7 (DO_ASSEMBLED_COMBAT to MKII via the instance GUID
-- + the stored VX-001 / ACU refs) are ported.
local function vol7ronActivation7(creature, guid)
    local st = bossState[guid]
    if not st then
        return
    end
    -- C++ :638-640 loops DATA_LEVIATHAN_MK_II..DATA_AERIAL_COMMAND_UNIT
    -- (32..34); MKII resolves via the instance GUID (INTRO_2/3 twin).
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        mkii:DoAction(DO_ASSEMBLED_COMBAT)
    end
    if st.vx001 then
        st.vx001:DoAction(DO_ASSEMBLED_COMBAT)
    end
    if st.aerial then
        st.aerial:DoAction(DO_ASSEMBLED_COMBAT)
    end
end

local function vol7ronActivation6(creature, guid)
    local st = bossState[guid]
    local vx001 = st and st.vx001
    if vx001 then
        creature:CastSpell(vx001, SPELL_SEAT_2)
    end
    schedule(guid, "vol7ronact7", 5000, function()
        vol7ronActivation7(creature, guid)
    end)
end

local function vol7ronActivation5(creature, guid)
    creature:Talk(SAY_V07TRON_ACTIVATE)
    schedule(guid, "vol7ronact6", 3000, function()
        vol7ronActivation6(creature, guid)
    end)
end

local function vol7ronActivation4(creature, guid)
    local st = bossState[guid]
    local aerial = st and st.aerial
    local vx001 = st and st.vx001
    if aerial and vx001 then
        aerial:CastSpell(vx001, SPELL_MOUNT_VX_001)
        aerial:CastSpell(aerial, SPELL_HALF_HEAL)
    end
    schedule(guid, "vol7ronact5", 4000, function()
        vol7ronActivation5(creature, guid)
    end)
end

local function vol7ronActivation3(creature, guid)
    -- C++ :611-614: MKII MovePoint to WP_MKII_P4_POS_4 via the MoveTo
    -- twin (the P4_POS_4 arrival below schedules MOVE_POINT_5).
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        mkii:MoveTo(WP_MKII_P4_POS_4, MKII_P4_POS_4[1], MKII_P4_POS_4[2], MKII_P4_POS_4[3])
    end
    schedule(guid, "vol7ronact4", 5000, function()
        vol7ronActivation4(creature, guid)
    end)
end

local function vol7ronActivation2(creature, guid)
    -- C++ :600-609: MKII + VX-001 both required (nested instance-GUID
    -- lookups; the VX-001 ref is the stored summon ref, the DATA_VX_001
    -- twin). VX-001 drops TORSO_DISABLED, then mounts MKII.
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    local st = bossState[guid]
    local vx001 = st and st.vx001
    if mkii and vx001 then
        vx001:RemoveAura(SPELL_TORSO_DISABLED)
        vx001:CastSpell(mkii, SPELL_MOUNT_MKII)
    end
    schedule(guid, "vol7ronact3", 4500, function()
        vol7ronActivation3(creature, guid)
    end)
end

local function vol7ronActivation1(creature, guid)
    -- C++ :595-598: MKII SetFacingTo(float(M_PI)).
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        mkii:SetFacingTo(3.141593)
    end
    schedule(guid, "vol7ronact2", 1000, function()
        vol7ronActivation2(creature, guid)
    end)
end

-- C++ DoAction (:397-422, event 38): the phase-machine receiver. All
-- five senders are live now -- MKII MovementInform :807 (P1 idle
-- arrival, tick 561) and :817 (P4_POS_3 arrival, tick 561), VX-001
-- DamageTaken :960 (tick 561), ACU MovementInform :1229 (tick 561),
-- and DO_ACTIVATE_HARD_MODE from the computer self-destruct :1453
-- (npc_mimiron_computer.lua, this tick).
local function mimironDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == DO_ACTIVATE_VX001 then
        schedule(guid, "vx001act1", 1000, function()
            vx001Activation1(creature, guid)
        end)
    elseif action == DO_ACTIVATE_AERIAL then
        schedule(guid, "aerialact1", 5000, function()
            aerialActivation1(creature, guid)
        end)
    elseif action == DO_ACTIVATE_V0L7R0N_1 then
        -- C++ :407-411: Talk SAY_AERIAL_DEATH, then MKII MovePoint to
        -- WP_MKII_P4_POS_1 via the MoveTo twin (the P4_POS_1 arrival
        -- below schedules EVENT_MOVE_POINT_2).
        creature:Talk(SAY_AERIAL_DEATH)
        local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
        if mkii then
            mkii:MoveTo(WP_MKII_P4_POS_1, MKII_P4_POS_1[1], MKII_P4_POS_1[2], MKII_P4_POS_1[3])
        end
    elseif action == DO_ACTIVATE_V0L7R0N_2 then
        schedule(guid, "vol7ronact1", 1000, function()
            vol7ronActivation1(creature, guid)
        end)
    elseif action == DO_ACTIVATE_HARD_MODE then
        stateFor(guid).fireFighter = true
        creature:DoZoneInCombat()
    end
end

RegisterCreatureEvent(NPC_MIMIRON, 5, mimironSpawn)
RegisterCreatureEvent(NPC_MIMIRON, 23, mimironReset)
RegisterCreatureEvent(NPC_MIMIRON, 1, mimironEnterCombat)
RegisterCreatureEvent(NPC_MIMIRON, 4, mimironDied)
RegisterCreatureEvent(NPC_MIMIRON, 38, mimironDoAction)

-- C++ spell_mimiron_summon_flames_spread_AuraScript::HandleTick
-- (boss_mimiron.cpp:2615-2627): PreventDefaultAction; if the aura
-- caster has SPELL_FLAMES_PERIODIC_TRIGGER (64561) it casts
-- SPELL_SUMMON_FLAMES_SPREAD (64564) triggered on the aura target.
-- Returns true, the PreventDefaultAction twin, so the Go default
-- trigger cast does not also fire.
local function summonFlamesSpreadTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster and caster:HasAura(SPELL_FLAMES_PERIODIC_TRIGGER) then
        caster:CastSpell(target, SPELL_SUMMON_FLAMES_SPREAD, true)
    end
    return true
end

RegisterAuraEvent(SPELL_SUMMON_FLAMES_SPREAD_TRIGGER, 1, summonFlamesSpreadTick)

-- C++ spell_mimiron_weld_AuraScript::HandleTick
-- (boss_mimiron.cpp:2760-2772): the aura target (the player riding in
-- Mimiron's vehicle) casts one of the four RepairSpells on the vehicle
-- base every fifth tick (GetTickNumber() % 5 == 0) and faces the
-- vehicle every tick. GetVehicleBase has no direct binding; the
-- passenger twin is target:IsOnVehicle():GetOwner() — the Vehicle
-- object the player rides, whose owner is the base unit
-- (lua_player.go IsOnVehicle, lua_vehicle.go GetOwner). The repair
-- cast is non-triggered (C++ passes no triggered flag) and rides the
-- player session path like every other non-triggered player cast. No
-- PreventDefaultAction: the handler runs alongside the Go default
-- trigger cast, like C++.
local SPELL_WELD_REPAIR_SPELLS = { 52391, 63313, 63314, 63316 } -- boss_mimiron.cpp:334-340 (SPELL_SEAT_1/2/3/5)

local function weldTick(event, aura, target)
    local vehicle = target:IsOnVehicle()
    if vehicle then
        local base = vehicle:GetOwner()
        if base then
            if aura:GetTickNumber() % 5 == 0 then
                target:CastSpell(base, SPELL_WELD_REPAIR_SPELLS[math.random(1, 4)])
            end
            target:SetFacingToObject(base)
        end
    end
end

RegisterAuraEvent(SPELL_WELD, 1, weldTick)
