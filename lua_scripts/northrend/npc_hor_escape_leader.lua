-- Jaina / Sylvanas escape leader (Halls of Reflection) — Lua port of
-- npc_jaina_or_sylvanas_escape_hor in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (:825-1182; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entries: 36955 NPC_JAINA_ESCAPE / 37554 NPC_SYLVANAS_ESCAPE
-- (halls_of_reflection.h:82-83). No RegisterLuaBoss line needed —
-- entry-keyed dispatch (non-boss NPC precedent).
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arms):
-- Reset (:840-846) — _icewall = 0, EVENT_ESCAPE in 1s; rides ev23
--   (fires on spawn and on evade, the gauntlet precedent). The
--   DoStopTimedAchievement leg has no Lua bridge (session-internal
--   achievements.go) and stays out. _prefight / _invincibility are
--   NOT reset here (C++ Reset leaves them; ctor-false/true only).
--   No ev2 registration: Go fires ev23 BEFORE ev2 on evade
--   (triggerCreatureEvade, creaturemotion.go:4697-4699), so an ev2
--   timer-cancel would eat the EVENT_ESCAPE the Reset leg just
--   scheduled; the ev23 leg already cancels the chain timers.
-- JustDied (:843-847) — the LK EnterEvadeMode relay (FAIL legs),
--   ported earlier; this tick adds the chain-timer teardown.
-- DamageTaken (:849-853) — the _invincibility clamp via ev9 (the
--   LK-escape ev9 precedent): lethal damage becomes health-1 while
--   invincible; cleared at EVENT_ESCAPE_6. A missing state table
--   reads invincible (the ctor default).
-- DoAction (:864-882) — ev38 receivers for ACTION_START_PREFIGHT
--   (-668002), ACTION_WALL_BROKEN (-668003), ACTION_GUNSHIP_ARRIVAL
--   (-668004), ACTION_GUNSHIP_ARRIVAL_2 (-668005). Fire sites:
--   START_PREFIGHT rides the at_hor_shadow_throne trigger (the C++
--   instance SetBossState IN_PROGRESS override leg,
--   instance_halls_of_reflection.cpp:384-388 — the trigger writes
--   IN_PROGRESS and sends the DoAction, the HOR_SETDATA_NEXT_WAVE
--   Go-only-signal precedent); WALL_BROKEN rides the escape-trash
--   death check (npc_hor_escape_trash.lua); the gunship arrivals
--   have no Lua fire site (instance ProcessEvent
--   EVENT_GUNSHIP_ARRIVAL, transport arrival) — the receivers are
--   registered, the triggers stay out.
-- OnGossipSelect (:901-914) — list id 0: close gossip
--   (GossipClearMenu + GossipComplete, the opera precedent),
--   RemoveFlag GOSSIP, EVENT_ESCAPE_6 at once (C++ 0s; 1ms twin).
--   OnGossipHello (:884-899) stays out: the DONE final menu
--   (10930/10931) cannot be sent — GossipSendMenu carries no
--   DB-loaded options (buildGossipMessage, gossip.go:1598) and the
--   option text lives only in gossip_menu_option rows the repo's
--   world.db placeholder does not ship; returning false from a
--   hello hook is identical to no hook (the default menu loads),
--   so no hello hook is registered.
-- UpdateAI event machine (:1000-1150) — EVENT_ESCAPE (:1002-1021)
--   through EVENT_ESCAPE_17 (:1140-1149), driven by CreateLuaEvent
--   one-shots on the bronjahm convention. Movement legs ride
--   creature:MoveTo + the ev28 MovementInform arrival gate
--   (lua_creature_events.go:4974, runtime.go:389-397; the nether
--   drake precedent) — the old "no MovePoint bridge" claim is
--   stale and retracted. Cross-creature legs read the LK from
--   instanceData.lichkingEscape (the instance OnCreatureCreate
--   bind); the team read is instanceData.teamInInstance (the
--   instance OnPlayerEnter latch, 0 Alliance / 1 Horde, nil =
--   Alliance like the C++ GetData default).
--   EVENT_ESCAPE: barrier/cloak self-cast by team, taunt the LK
--   (69857 triggered) + ApplySpellImmune taunt/attack-me rows,
--   mutual EngageWithTarget, SetHealth(252000), RemoveFlag
--   gossip|questgiver.
--   EVENT_ESCAPE_1: LK Talk (0 Alliance / 1 Horde), E2 in 8s.
--   EVENT_ESCAPE_2: AttackStop + StopMoving + REACT_PASSIVE,
--   team prison/retreat cast, LK REACT_PASSIVE + PACIFIED flag,
--   E3 in 1500ms.
--   EVENT_ESCAPE_3: Horde-only self-cast dark binding (triggered),
--   E4 in 1s.
--   EVENT_ESCAPE_4: credit cast + Talk(0); LK SetImmuneToPC
--   (the UNIT_FLAG_IMMUNE_TO_PC 0x100 flag half — C++
--   Unit::SetImmuneToPC is that flag plus combat-ref ending,
--   Unit.cpp:8453; the ref-ending half has no bridge) +
--   RemoveAllAttackers; the DeleteAllFromThreatList leg has no
--   bridge. E5 in 2s.
--   EVENT_ESCAPE_5: MoveTo shadow-throne door (id 1).
--   MovementInform POINT_SHADOW_THRONE_DOOR (1): remove the
--   barrier/cloak aura, SetFlag GOSSIP, SetHealth(252000),
--   SetFacingTo(2.687807).
--   EVENT_ESCAPE_6: LK RemoveFlag PACIFIED + SetImmuneToPC(false)
--   (flag half), LK stun-break self-cast + RemoveAura prison/
--   binding by team; _invincibility = false; the
--   DoStartTimedAchievement leg has no bridge. E7 in 1s.
--   EVENT_ESCAPE_7: LK roar emote; leader MoveTo route[0]; E8 3s.
--   EVENT_ESCAPE_8: LK MoveTo route[0]; E9 in 1s.
--   EVENT_ESCAPE_9: leader MoveTo route[1]; E10 in 5s.
--   EVENT_ESCAPE_10: leader MoveTo route[2]; LK MoveTo(1, first
--   summon); E11 in 6s.
--   EVENT_ESCAPE_11: SummonIceWall; E12 in 4s.
--   EVENT_ESCAPE_12: LK self-cast pain and suffering (triggered);
--   leader MoveTo POINT_ATTACK_ICEWALL route[3].
--   MovementInform POINT_ATTACK_ICEWALL (2): AttackIceWall —
--   Talk(1 + _icewall), face the wall target, team destroy-wall
--   self-cast (triggered).
--   EVENT_ESCAPE_13 (WALL_BROKEN): DestroyIceWall; MoveTo
--   POINT_ATTACK_ICEWALL route[_icewall + 3]; E14 in 8s.
--   EVENT_ESCAPE_14: SummonIceWall.
--   EVENT_ESCAPE_15 (final wall): DestroyIceWall; Talk(5); LK
--   MoveTo(2, final pos) + RemoveAura remorseless winter; leader
--   MoveTo POINT_TRAP route[7].
--   MovementInform POINT_TRAP (3): Talk(7), face the LK.
--   EVENT_ESCAPE_16: RemoveAura harvest soul; Alliance Talk(8);
--   SetBossState(DONE). The gunship EnableMovement leg has no
--   transport bridge.
--   EVENT_ESCAPE_17: Talk(9 Alliance / 8 Horde); credit cast;
--   SetFlag gossip|questgiver.
-- SummonIceWall (:955-970): LK StopMoving; summon the wall target
--   (37014, Go type 7 MANUAL_DESPAWN, 12min) at
--   IceWallTargetPosition[_icewall]; LK casts 69768 on it; then
--   LK SetData(DATA_ICEWALL 11, _icewall) + the Go-only DoAction
--   (ACTION_LK_ICEWALL -668010) that fires the LK's SetData
--   machine in npc_the_lich_king_escape.lua (creature:SetData is
--   a plain store, not a dispatch — lua_creature_events.go:784 —
--   so the store carries the value and the DoAction is the
--   notification, the HOR_SETDATA_NEXT_WAVE pattern).
-- DestroyIceWall (:941-953): RemoveAura destroy-wall spell by
--   team; the wall-target despawn ports. The HandleGameObject +
--   5s delayed GO Delete legs stay out (no GO-state/delete
--   bridge): the SPELL_SUMMON_ICE_WALL GO remains standing.
--
-- DOCUMENTED-ONLY:
-- The LK MovementInform point-2 gunship leg and the instance
-- DONE override legs (chest flag, cave-in door, LK 72830 cast)
-- ride the transport/GO bridges that do not exist (see the
-- instance file header).
-- The leader's C++ UpdateAI DoMeleeAttackIfReady is engine-driven
-- in Go, like every other port.

local ENTRY_JAINA_ESCAPE = 36955 -- halls_of_reflection.h:82
local ENTRY_SYLVANAS_ESCAPE = 37554 -- halls_of_reflection.h:83
local NPC_THE_LICH_KING_ESCAPE = 36954 -- halls_of_reflection.h:84
local NPC_ICE_WALL_TARGET = 37014 -- halls_of_reflection.h:85

local DATA_THE_LICH_KING_ESCAPE = 2 -- halls_of_reflection.h:39
local DATA_ICEWALL = 11 -- halls_of_reflection.h:46

local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local ALLIANCE = 0 -- TeamId TEAM_ALLIANCE; instanceData.teamInInstance
local JAINA_SYLVANAS_MAX_HEALTH = 252000 -- halls_of_reflection.cpp:210

local POINT_SHADOW_THRONE_DOOR = 1 -- halls_of_reflection.cpp:213
local POINT_ATTACK_ICEWALL = 2
local POINT_TRAP = 3
local POINT_MOTION_TYPE = 8 -- MovementDefines.h (nether drake precedent)

local ACTION_START_PREFIGHT = -668002 -- halls_of_reflection.h:130
local ACTION_WALL_BROKEN = -668003
local ACTION_GUNSHIP_ARRIVAL = -668004
local ACTION_GUNSHIP_ARRIVAL_2 = -668005
local ACTION_LK_ICEWALL = -668010 -- Go-only: fires the LK SetData machine

local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (baltharus precedent)
local UNIT_NPC_FLAG_GOSSIP = 0x00000001 -- UnitDefines.h:186
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002 -- UnitDefines.h:187
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h (algalon precedent)
local UNIT_FLAG_PACIFIED = 0x00020000 -- UnitDefines.h:141
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132

local REACT_PASSIVE = 0 -- UnitDefines.h:340 (falric precedent)

local IMMUNITY_EFFECT = 0 -- SharedDefines.h:1426 (enum SpellEffects)
local IMMUNITY_STATE = 1 -- SharedDefines.h:1427 (enum AuraType)
local SPELL_AURA_MOD_TAUNT = 11 -- SpellAuraDefines.h:91
local SPELL_EFFECT_ATTACK_ME = 114 -- SharedDefines.h:925

local SPELL_TAUNT_ARTHAS = 69857 -- halls_of_reflection.cpp:238
local SPELL_JAINA_ICE_BARRIER = 69787
local SPELL_JAINA_ICE_PRISON = 69708
local SPELL_JAINA_DESTROY_ICE_WALL = 69784
local SPELL_SYLVANAS_CLOAK_OF_DARKNESS = 70188
local SPELL_SYLVANAS_DARK_BINDING = 70194
local SPELL_SYLVANAS_DESTROY_ICE_WALL = 70224
local SPELL_SYLVANAS_BLINDING_RETREAT = 70199
local SPELL_CREDIT_FINDING_JAINA = 71538 -- halls_of_reflection.h:164
local SPELL_CREDIT_FINDING_SYLVANAS = 71536
local SPELL_CREDIT_ESCAPING_ARTHAS = 71352
local SPELL_STUN_BREAK_JAINA = 69764 -- halls_of_reflection.cpp:260
local SPELL_STUN_BREAK_SYLVANAS = 70200
local SPELL_PAIN_AND_SUFFERING = 74115
local SPELL_HARVEST_SOUL = 69866
local SPELL_SUMMON_ICE_WALL = 69768
local SPELL_REMORSELESS_WINTER = 69780

local SAY_JAINA_SYLVANAS_ESCAPE_1 = 0 -- halls_of_reflection.cpp:108
local SAY_JAINA_SYLVANAS_ESCAPE_2 = 1
local SAY_JAINA_SYLVANAS_ESCAPE_6 = 5
local SAY_JAINA_SYLVANAS_ESCAPE_8 = 7
local SAY_JAINA_ESCAPE_9 = 8
local SAY_JAINA_ESCAPE_10 = 9
local SAY_SYLVANAS_ESCAPE_9 = 8
local SAY_LK_ESCAPE_1 = 0 -- halls_of_reflection.cpp:121
local SAY_LK_ESCAPE_2 = 1

local SOUND_LK_FURY_OF_FROSTMOURNE = 17224 -- halls_of_reflection.cpp:215
local SPELL_FURY_OF_FROSTMOURNE = 70063 -- halls_of_reflection.cpp:243

local EMOTE_ONESHOT_ROAR = 15 -- SharedDefines.h:1950

local SUMMON_TYPE_MANUAL_DESPAWN = 7 -- Go numbering (creaturemotion.go:501-510)

-- C++ NpcJainaOrSylvanasEscapeRoute[9] (halls_of_reflection.cpp:271-281),
-- Lua 1-indexed; MoveTo takes x/y/z only, orientations live in C++.
local ESCAPE_ROUTE = {
    { 5601.217285, 2207.652832, 731.541931 }, -- [1] C++ route[0], leave the throne room
    { 5607.224375, 2173.913330, 731.126038 }, -- [2] C++ route[1], adjust route
    { 5583.427246, 2138.784180, 731.150391 }, -- [3] C++ route[2], stop for talking
    { 5560.281738, 2104.025635, 731.410889 }, -- [4] C++ route[3], attack the first icewall
    { 5510.990723, 2000.772217, 734.716064 }, -- [5] C++ route[4], attack the second icewall
    { 5452.641113, 1905.762329, 746.530579 }, -- [6] C++ route[5], attack the third icewall
    { 5338.126953, 1768.429810, 767.237244 }, -- [7] C++ route[6], attack the fourth icewall
    { 5259.06, 1669.27, 784.3008 }, -- [8] C++ route[7], trap (sniffed)
    { 5265.53, 1681.6, 784.2947 }, -- [9] C++ route[8], final position (sniffed)
}

-- C++ SylvanasShadowThroneDoorPosition (halls_of_reflection.cpp:330).
local SHADOW_THRONE_DOOR_POS = { 5576.79, 2235.73, 733.0029 }
local SHADOW_THRONE_DOOR_O = 2.687807

-- C++ IceWallTargetPosition[4] (halls_of_reflection.cpp:332-338),
-- Lua 1-indexed; SummonCreature takes x/y/z/o.
local ICE_WALL_TARGET_POS = {
    { 5547.833, 2083.701, 731.4332, 1.029744 }, -- [1] C++ [0], 1st icewall
    { 5503.213, 1969.547, 737.0245, 1.27409 }, -- [2] C++ [1], 2nd icewall
    { 5439.976, 1879.005, 752.7048, 1.064651 }, -- [3] C++ [2], 3rd icewall
    { 5318.289, 1749.184, 771.9423, 0.8726646 }, -- [4] C++ [3], 4th icewall
}

-- C++ LichKingFirstSummon / LichKingFinalPos (halls_of_reflection.cpp:290-291).
local LICH_KING_FIRST_SUMMON_POS = { 5600.076172, 2192.270996, 731.750488 }
local LICH_KING_FINAL_POS = { 5283.742188, 1706.335693, 783.293518 }

local timers = {}
local leaderState = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function alive(guid)
    return timers[guid] ~= nil
end

local function getInstanceData(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

local function getLichking(creature)
    local data = getInstanceData(creature)
    if not data then
        return nil
    end
    return data.lichkingEscape
end

local function isAlliance(creature)
    local data = getInstanceData(creature)
    if not data then
        return true
    end
    return (data.teamInInstance or ALLIANCE) == ALLIANCE
end

local function getState(guid)
    local st = leaderState[guid]
    if not st then
        st = { icewall = 0, prefight = false, invincible = true }
        leaderState[guid] = st
    end
    return st
end

-- C++ SummonIceWall :955-970: LK StopMoving, summon the wall target,
-- LK casts 69768 on it, then the LK SetData(DATA_ICEWALL) leg —
-- the store write + Go-only DoAction pair (see header).
local function summonIceWall(creature, guid)
    local st = getState(guid)
    if st.icewall >= 4 then
        return
    end
    local lichking = getLichking(creature)
    if not lichking then
        return
    end
    lichking:StopMoving()
    local pos = ICE_WALL_TARGET_POS[st.icewall + 1]
    local wallTarget = creature:SummonCreature(NPC_ICE_WALL_TARGET, pos[1], pos[2], pos[3], pos[4], SUMMON_TYPE_MANUAL_DESPAWN, 720000)
    if wallTarget then
        lichking:CastSpell(wallTarget, SPELL_SUMMON_ICE_WALL)
    end
    lichking:SetData(DATA_ICEWALL, st.icewall)
    lichking:DoAction(ACTION_LK_ICEWALL)
end

-- C++ DestroyIceWall :941-953: RemoveAura destroy-wall spell by team,
-- wall-target despawn. The HandleGameObject + 5s delayed GO Delete
-- legs stay out (no GO-state/delete bridge).
local function destroyIceWall(creature)
    if isAlliance(creature) then
        creature:RemoveAura(SPELL_JAINA_DESTROY_ICE_WALL)
    else
        creature:RemoveAura(SPELL_SYLVANAS_DESTROY_ICE_WALL)
    end
    local data = getInstanceData(creature)
    if data and data.icewallTarget then
        data.icewallTarget:DespawnOrUnsummon()
    end
end

-- C++ AttackIceWall :972-985: Talk(SAY_2 + _icewall) while a wall
-- stands, face the wall target, team destroy-wall self-cast.
local function attackIceWall(creature, guid)
    local st = getState(guid)
    if st.icewall < 4 then
        creature:Talk(SAY_JAINA_SYLVANAS_ESCAPE_2 + st.icewall)
    end
    local data = getInstanceData(creature)
    if data and data.icewallTarget then
        creature:SetFacingToObject(data.icewallTarget)
    end
    if isAlliance(creature) then
        creature:CastSpell(creature, SPELL_JAINA_DESTROY_ICE_WALL, true)
    else
        creature:CastSpell(creature, SPELL_SYLVANAS_DESTROY_ICE_WALL, true)
    end
end

-- C++ UpdateAI event machine :1000-1150. One dispatch function (no
-- forward references): id 0 = EVENT_ESCAPE, 1-17 = EVENT_ESCAPE_1..17.
-- No IsCasting gate: the C++ leader UpdateAI has no CASTING pre-loop
-- return (unlike the LK's, which the LK file ports).
local function runEscapeEvent(creature, guid, id)
    if not alive(guid) then
        return
    end
    local st = getState(guid)
    if id == 0 then -- EVENT_ESCAPE :1002-1021
        if isAlliance(creature) then
            creature:CastSpell(creature, SPELL_JAINA_ICE_BARRIER)
        else
            creature:CastSpell(creature, SPELL_SYLVANAS_CLOAK_OF_DARKNESS)
        end
        local lichking = getLichking(creature)
        if lichking then
            creature:CastSpell(lichking, SPELL_TAUNT_ARTHAS, true)
            lichking:ApplySpellImmune(0, IMMUNITY_STATE, SPELL_AURA_MOD_TAUNT, true)
            lichking:ApplySpellImmune(0, IMMUNITY_EFFECT, SPELL_EFFECT_ATTACK_ME, true)
            creature:EngageWithTarget(lichking)
            lichking:EngageWithTarget(creature)
        end
        creature:SetHealth(JAINA_SYLVANAS_MAX_HEALTH)
        creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP + UNIT_NPC_FLAG_QUESTGIVER)
    elseif id == 1 then -- EVENT_ESCAPE_1 :1022-1034
        local lichking = getLichking(creature)
        if lichking then
            if isAlliance(creature) then
                lichking:Talk(SAY_LK_ESCAPE_1)
            else
                lichking:Talk(SAY_LK_ESCAPE_2)
            end
            schedule(guid, "e2", 8000, function() runEscapeEvent(creature, guid, 2) end)
        end
    elseif id == 2 then -- EVENT_ESCAPE_2 :1035-1056
        creature:AttackStop()
        creature:StopMoving()
        creature:SetReactState(REACT_PASSIVE)
        if isAlliance(creature) then
            creature:CastSpell(creature, SPELL_JAINA_ICE_PRISON, false)
        else
            creature:CastSpell(creature, SPELL_SYLVANAS_BLINDING_RETREAT, true)
        end
        local lichking = getLichking(creature)
        if lichking then
            lichking:SetReactState(REACT_PASSIVE)
            lichking:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
        end
        schedule(guid, "e3", 1500, function() runEscapeEvent(creature, guid, 3) end)
    elseif id == 3 then -- EVENT_ESCAPE_3 :1057-1061
        if not isAlliance(creature) then
            creature:CastSpell(creature, SPELL_SYLVANAS_DARK_BINDING, true)
        end
        schedule(guid, "e4", 1000, function() runEscapeEvent(creature, guid, 4) end)
    elseif id == 4 then -- EVENT_ESCAPE_4 :1062-1081
        if isAlliance(creature) then
            creature:CastSpell(creature, SPELL_CREDIT_FINDING_JAINA)
        else
            creature:CastSpell(creature, SPELL_CREDIT_FINDING_SYLVANAS)
        end
        creature:Talk(SAY_JAINA_SYLVANAS_ESCAPE_1)
        local lichking = getLichking(creature)
        if lichking then
            -- SetImmuneToPC(true): the flag half (Unit.cpp:8453); the
            -- combat-ref ending half has no bridge. RemoveAllAttackers
            -- ports; DeleteAllFromThreatList stays out.
            lichking:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            lichking:RemoveAllAttackers()
        end
        schedule(guid, "e5", 2000, function() runEscapeEvent(creature, guid, 5) end)
    elseif id == 5 then -- EVENT_ESCAPE_5 :1082-1084
        creature:MoveTo(POINT_SHADOW_THRONE_DOOR, SHADOW_THRONE_DOOR_POS[1], SHADOW_THRONE_DOOR_POS[2], SHADOW_THRONE_DOOR_POS[3])
    elseif id == 6 then -- EVENT_ESCAPE_6 :1085-1106
        local lichking = getLichking(creature)
        if lichking then
            lichking:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
            lichking:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            if isAlliance(creature) then
                lichking:CastSpell(lichking, SPELL_STUN_BREAK_JAINA)
                lichking:RemoveAura(SPELL_JAINA_ICE_PRISON)
            else
                lichking:CastSpell(lichking, SPELL_STUN_BREAK_SYLVANAS)
                lichking:RemoveAura(SPELL_SYLVANAS_DARK_BINDING)
            end
        end
        st.invincible = false
        -- DoStartTimedAchievement stays out (no bridge).
        schedule(guid, "e7", 1000, function() runEscapeEvent(creature, guid, 7) end)
    elseif id == 7 then -- EVENT_ESCAPE_7 :1107-1113
        local lichking = getLichking(creature)
        if lichking then
            lichking:PerformEmote(EMOTE_ONESHOT_ROAR)
        end
        local pos = ESCAPE_ROUTE[1]
        creature:MoveTo(0, pos[1], pos[2], pos[3])
        schedule(guid, "e8", 3000, function() runEscapeEvent(creature, guid, 8) end)
    elseif id == 8 then -- EVENT_ESCAPE_8 :1114-1118
        local lichking = getLichking(creature)
        if lichking then
            local pos = ESCAPE_ROUTE[1]
            lichking:MoveTo(0, pos[1], pos[2], pos[3])
        end
        schedule(guid, "e9", 1000, function() runEscapeEvent(creature, guid, 9) end)
    elseif id == 9 then -- EVENT_ESCAPE_9 :1119-1122
        local pos = ESCAPE_ROUTE[2]
        creature:MoveTo(0, pos[1], pos[2], pos[3])
        schedule(guid, "e10", 5000, function() runEscapeEvent(creature, guid, 10) end)
    elseif id == 10 then -- EVENT_ESCAPE_10 :1123-1129
        local pos = ESCAPE_ROUTE[3]
        creature:MoveTo(0, pos[1], pos[2], pos[3])
        local lichking = getLichking(creature)
        if lichking then
            local lkPos = LICH_KING_FIRST_SUMMON_POS
            lichking:MoveTo(1, lkPos[1], lkPos[2], lkPos[3])
        end
        schedule(guid, "e11", 6000, function() runEscapeEvent(creature, guid, 11) end)
    elseif id == 11 then -- EVENT_ESCAPE_11 :1130-1133
        summonIceWall(creature, guid)
        schedule(guid, "e12", 4000, function() runEscapeEvent(creature, guid, 12) end)
    elseif id == 12 then -- EVENT_ESCAPE_12 :1134-1139
        local lichking = getLichking(creature)
        if lichking then
            lichking:CastSpell(lichking, SPELL_PAIN_AND_SUFFERING, true)
        end
        local pos = ESCAPE_ROUTE[4]
        creature:MoveTo(POINT_ATTACK_ICEWALL, pos[1], pos[2], pos[3])
    elseif id == 13 then -- EVENT_ESCAPE_13 :1140-1146 (ICEWALL BROKEN)
        destroyIceWall(creature)
        if st.icewall ~= 0 and st.icewall < 4 then
            local pos = ESCAPE_ROUTE[st.icewall + 4]
            creature:MoveTo(POINT_ATTACK_ICEWALL, pos[1], pos[2], pos[3])
        end
        schedule(guid, "e14", 8000, function() runEscapeEvent(creature, guid, 14) end)
    elseif id == 14 then -- EVENT_ESCAPE_14 :1147-1149
        summonIceWall(creature, guid)
    elseif id == 15 then -- EVENT_ESCAPE_15 :1150-1161 (FINAL PART)
        destroyIceWall(creature)
        creature:Talk(SAY_JAINA_SYLVANAS_ESCAPE_6)
        local lichking = getLichking(creature)
        if lichking then
            local lkPos = LICH_KING_FINAL_POS
            lichking:MoveTo(2, lkPos[1], lkPos[2], lkPos[3])
            lichking:RemoveAura(SPELL_REMORSELESS_WINTER)
        end
        local pos = ESCAPE_ROUTE[8]
        creature:MoveTo(POINT_TRAP, pos[1], pos[2], pos[3])
    elseif id == 16 then -- EVENT_ESCAPE_16 :1162-1171
        creature:RemoveAura(SPELL_HARVEST_SOUL)
        if isAlliance(creature) then
            creature:Talk(SAY_JAINA_ESCAPE_9)
        end
        -- gunship->EnableMovement(true) stays out (no transport bridge).
        creature:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_DONE)
    elseif id == 17 then -- EVENT_ESCAPE_17 :1172-1179
        if isAlliance(creature) then
            creature:Talk(SAY_JAINA_ESCAPE_10)
        else
            creature:Talk(SAY_SYLVANAS_ESCAPE_9)
        end
        creature:CastSpell(creature, SPELL_CREDIT_ESCAPING_ARTHAS)
        creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP + UNIT_NPC_FLAG_QUESTGIVER)
    end
end

-- C++ Reset (:840-846): _events.Reset() (the timer cancel) +
-- _icewall = 0 + EVENT_ESCAPE in 1s. _prefight / _invincibility are
-- left alone (see header). Fires on spawn and on evade.
local function leaderReset(event, creature)
    local guid = creature:GetGUIDLow()
    local st = getState(guid)
    st.icewall = 0
    cancelTimers(guid)
    schedule(guid, "escape", 1000, function() runEscapeEvent(creature, guid, 0) end)
end

-- C++ JustDied (:843-847) + the relayed EnterEvadeMode legs
-- (:1305-1316, C++ order: instance SetBossState(FAIL) -> StopMoving ->
-- DoPlaySoundToSet -> DoCastAOE (self-cast idiom) -> DespawnOrUnsummon(12s)).
-- Do NOT call lichking:EnterEvadeMode — no such creature method exists.
local function escapeLeaderDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    leaderState[guid] = nil
    local mapObj = creature:GetMap()
    if not mapObj then return end
    local data = mapObj:GetInstanceData()
    if not data or not data.lichkingEscape then return end
    local lichking = data.lichkingEscape
    if lichking:GetBossState(DATA_THE_LICH_KING_ESCAPE) == BOSS_STATE_FAIL then
        return -- LK already evaded; mirrors the C++ _despawn once-latch
    end
    lichking:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_FAIL)
    lichking:StopMoving()
    lichking:PlayDirectSound(SOUND_LK_FURY_OF_FROSTMOURNE)
    lichking:CastSpell(lichking, SPELL_FURY_OF_FROSTMOURNE)
    CreateLuaEvent(function() lichking:Despawn() end, 12000)
end

-- C++ DamageTaken (:849-853): lethal damage becomes health-1 while
-- _invincibility holds. Event 9 fires pre-application, so
-- creature:GetHealth() is the pre-damage value, exactly like C++.
local function leaderDamageTaken(event, creature, attacker, damage)
    local st = leaderState[creature:GetGUIDLow()]
    if st == nil or st.invincible then
        local health = creature:GetHealth()
        if damage >= health then
            return true, health - 1
        end
    end
end

-- C++ DoAction (:864-882). The _prefight once-latch is per-GUID state.
local function leaderDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local st = getState(guid)
    if action == ACTION_START_PREFIGHT then
        if st.prefight then
            return
        end
        st.prefight = true
        schedule(guid, "e1", 1000, function() runEscapeEvent(creature, guid, 1) end)
    elseif action == ACTION_WALL_BROKEN then
        st.icewall = st.icewall + 1
        if st.icewall < 4 then
            schedule(guid, "e13", 3000, function() runEscapeEvent(creature, guid, 13) end)
        else
            schedule(guid, "e15", 3000, function() runEscapeEvent(creature, guid, 15) end)
        end
    elseif action == ACTION_GUNSHIP_ARRIVAL then
        schedule(guid, "e16", 5000, function() runEscapeEvent(creature, guid, 16) end)
    elseif action == ACTION_GUNSHIP_ARRIVAL_2 then
        schedule(guid, "e17", 5000, function() runEscapeEvent(creature, guid, 17) end)
    end
end

-- C++ MovementInform (:987-1019): POINT_MOTION_TYPE only.
local function leaderMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    if pointId == POINT_SHADOW_THRONE_DOOR then
        if isAlliance(creature) then
            creature:RemoveAura(SPELL_JAINA_ICE_BARRIER)
        else
            creature:RemoveAura(SPELL_SYLVANAS_CLOAK_OF_DARKNESS)
        end
        creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
        creature:SetHealth(JAINA_SYLVANAS_MAX_HEALTH)
        creature:SetFacingTo(SHADOW_THRONE_DOOR_O)
    elseif pointId == POINT_ATTACK_ICEWALL then
        attackIceWall(creature, guid)
    elseif pointId == POINT_TRAP then
        creature:Talk(SAY_JAINA_SYLVANAS_ESCAPE_8)
        local lichking = getLichking(creature)
        if lichking then
            creature:SetFacingToObject(lichking)
        end
    end
end

-- C++ OnGossipSelect (:901-914): list id 0 closes the gossip,
-- clears the GOSSIP flag, and schedules EVENT_ESCAPE_6 at once.
-- C++ returns false (fall through to default handling); mirrored.
local function leaderGossipSelect(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
    local guid = creature:GetGUIDLow()
    schedule(guid, "e6", 1, function() runEscapeEvent(creature, guid, 6) end)
    return false
end

RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 4, escapeLeaderDied)
RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 9, leaderDamageTaken)
RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 23, leaderReset)
RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 28, leaderMovementInform)
RegisterCreatureEvent(ENTRY_JAINA_ESCAPE, 38, leaderDoAction)
RegisterCreatureGossipEvent(ENTRY_JAINA_ESCAPE, 2, leaderGossipSelect)

RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 4, escapeLeaderDied)
RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 9, leaderDamageTaken)
RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 23, leaderReset)
RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 28, leaderMovementInform)
RegisterCreatureEvent(ENTRY_SYLVANAS_ESCAPE, 38, leaderDoAction)
RegisterCreatureGossipEvent(ENTRY_SYLVANAS_ESCAPE, 2, leaderGossipSelect)
