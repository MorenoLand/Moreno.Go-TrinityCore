-- Blessed Banner (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_blessed_bannerAI — Reset() + EVENT_SPAWN + JustDied-drain arms). Zone-script unit
-- per northrend_script_loader.cpp order (howling_fjord done;
-- icecrown: npc_argent_valiant, npc_guardian_pavilion,
-- npc_tournament_training_dummy, npc_frostbrood_skytalon
-- documented-only, npc_blessed_banner ported).
-- Entry (BlessedBanner enum, verifiable from the C++ sources):
-- 30891 (NPC_BLESSED_BANNER). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arm (C++ Reset()): DoCast(SPELL_THREAT_PULSE 58113) —
-- the non-targeted self-cast — creature:CastSpell(creature,
-- spell) is the vaelastrasz self-cast convention — + Talk(
-- BANNER_SAY 0). The C++ Talk has no explicit target; the Go
-- Talk bridge takes a textId only (textId-only broadcast,
-- C++-exact for the line played — lecraft aggro-Talk
-- precedent). The banner never engages combat, so OnReset(23)
-- re-arms the same Reset fragment (C++ Reset semantics —
-- mushroom precedent); OnDied(4) carries the JustDied legs in C++ order
-- (zone_icecrown.cpp:455-459): Summons.DespawnAll() via the EVENT_SPAWN
-- latch, then the self-despawn.
-- Modeled in onSpawnOrReset: SetRegenerateHealth(false) (bridged —
-- lua_creature_events.go, honored by the Go regen tick). The ctor
-- SetCombatMovement(false) (zone_icecrown.cpp:421) is ported at
-- JustEngagedWith — the bridge's ctor-equivalent point (eredar_twins
-- convention).
-- Ported: EVENT_SPAWN 3s (DoSummon 31003 + 3x 30919 + 3x 30900,
-- CORPSE_TIMED_DESPAWN 30s = Lua type 5 + 30000ms, slad'ran precedent;
-- MovePoint walk-ins via the MoveTo twin; summons latched for the
-- JustDied drain; EVENT_INTRO_1's 15s schedule stays out below);
-- Unmodeled: EVENT_INTRO_1..3 Talk/facing chain (cross-creature-Talk /
-- facing bridges); EVENT_MASON_ACTION SetData(1,1) triggers
-- (no SetData dispatch path in the model); EVENT_START_FIGHT
-- LK(31013)-gated Talk 0 (world-search / cross-creature-Talk
-- bridges); EVENT_WAVE_SPAWN 10-20s summon waves of 30984 /
-- 30987 / 30986 with SetHomePosition + AttackStart on the
-- banner (summon / world-search / attack-start bridges);
-- EVENT_HALOF (30989 summon + LK_TALK_4); the PhaseCount == 8
-- victory machine (DoCast(me, SPELL_CRUSADERS_SPIRE_VICTORY
-- 58084) + DespawnEntry 30987/30986/30984/30989 + Dalfors
-- Talk(DALFORS_YELL_FINISHED 3) + EVENT_ENDED 10s despawn) —
-- all behind the movement / summon / world-search /
-- cross-creature-Talk / despawn bridges. 58084 + 58113 + 58121
-- + 30891 + 31003/30919/30900/30986/30984/30987/30989/31013
-- verifiable.
-- npc_argent_valiant — no NPC_ entry constant anywhere in the
-- C++ sources (the creature_template ScriptName binding is
-- DB-side, no TDB in this workspace — minigob precedent): no
-- Lua file written. The combat rotation (UpdateVictim -> charge
-- 63010 7s / shield-breaker 65147 10s via the omen 1s-pump
-- pattern) is stranded without an entry; the DamageTaken
-- duel-end machine (killing blow by a player -> damage = 0 +
-- pDoneBy->CastSpell(pDoneBy, 63049 kill credit, true) +
-- SetFaction(FACTION_FRIENDLY) + DespawnOrUnsummon(5s) +
-- SetHomePosition + EnterEvadeMode) now has its DamageTaken hook (ev9 rewrite), SetFaction,
-- Despawn, SetHomePosition bridges; the remaining blockers are
-- the missing entry, player self-cast 63049, and EnterEvadeMode; the ctor
-- MovePoint + SetFaction(FACTION_FRIENDLY) arm and the
-- MovementInform POINT_MOTION_TYPE -> SetFaction(FACTION_
-- MONSTER) flip sit behind the movement bridge (SetFaction IS
-- bridged — lua_creature_events.go:1125 — but the MovePoint /
-- MovementInform machine that drives the flip is not) —
-- documented-only. 63010/65147/63049 verifiable.
-- npc_guardian_pavilion — the trespasser machine is entirely
-- MoveInLineOfSight-driven (no proximity hook — Smeed /
-- mageguard_dalaran precedent): GetAreaId() != 4676 (Sunreaver)
-- && != 4677 (Silver Covenant) gate (area-ID bridge is live —
-- lua_creature_events.go — but the proximity / facing / team legs
-- below still block the machine), IsHostileTo + isInBackInMap(who, 5.0f)
-- gates (no proximity / facing bridges), HasAura(63987/63986)
-- trespasser gates (player:HasAura is bridged — lua_player.go:203;
-- still blocked by the proximity / area-ID / facing /
-- player-self-cast bridges),
-- GetTeamId() == TEAM_ALLIANCE branch (player:GetTeam IS bridged -
-- lua_player.go:87, teamForRace 0 = Alliance per achievement code;
-- the stale "no team bridge" note is retracted, the branch gate is
-- portable but the machine still stays out below),
-- who->CastSpell(who, 63987/63986, true)
-- self-cast on the player (no player-self-cast path from a
-- creature AI — corastrasza precedent) — documented-only. No
-- NPC_ entry constant in the C++ sources; 63987/63986/4676/
-- 4677 verifiable.
-- npc_tournament_training_dummy — ported in
-- northrend/npc_tournament_training_dummy.lua (all three
-- entries 33272/33229/33243): SpellHit credit machines,
-- DamageTaken damage=0 (event-9 rewrite — the old "no DamageTaken
-- hook" claim is retracted, lich_king_escape precedent), the
-- 33272 recast-defend leg, and the per-GUID vulnerable latch. The
-- 02:3x tick's duplicate block lived in this file and is removed
-- (it double-registered event 14 on 33272); this file keeps the
-- banner arms only.
-- npc_frostbrood_skytalon (VehicleAI) — the UpdateAI low-hp /
-- periodic arms run through VehicleAI::UpdateAI (the base is
-- unmodeled — wyrmrest_defender precedent); SpellHit 59335 ->
-- self-cast 54690 / 59319 -> DoCastAOE(59375) + EVENT_FLY_AWAY
-- 100ms (ev14 IS bridged — nagrand_banner precedent; the arms stay
-- out behind the unmodeled VehicleAI base and movement / summon
-- bridges); IsSummonedBy
-- MovePoint(POINT_GRAB_DECOY) + MovementInform -> DoCast(
-- summoner, 59318) behind the movement / summon bridges —
-- documented-only. No NPC_ entry constant in the C++ sources;
-- 59318/59375/54690/59335/59319 verifiable.

local SPELL_THREAT_PULSE = 58113

local BANNER_SAY = 0

local BLESSED_BANNER_ENTRY = 30891

local NPC_CRUSADER_LORD_DALFORS = 31003
local NPC_ARGENT_BATTLE_PRIEST = 30919
local NPC_ARGENT_MASON = 30900

-- C++ EVENT_SPAWN crew (zone_icecrown.cpp:465-514): {entry, spawnX,
-- spawnY, spawnZ, spawnO, walkX, walkY, walkZ} — [0] spawn + [1] walk-in
-- positions; [2] fight/action positions belong to the unported INTRO_3
-- leg and stay out.
local spawnTable = {
    {NPC_CRUSADER_LORD_DALFORS, 6458.703, 403.858, 490.498, 3.1205, 6422.950, 423.335, 510.451},
    {NPC_ARGENT_BATTLE_PRIEST, 6462.025, 403.681, 489.721, 3.1007, 6421.480, 423.576, 510.781},
    {NPC_ARGENT_BATTLE_PRIEST, 6463.969, 407.198, 489.240, 2.2689, 6419.778, 421.404, 510.972},
    {NPC_ARGENT_BATTLE_PRIEST, 6464.371, 400.944, 489.186, 6.1610, 6423.516, 425.782, 510.774},
    {NPC_ARGENT_MASON, 6462.929, 409.826, 489.392, 3.0968, 6428.163, 421.960, 508.297},
    {NPC_ARGENT_MASON, 6462.650, 405.670, 489.576, 2.9414, 6426.250, 419.194, 508.219},
    {NPC_ARGENT_MASON, 6462.646, 401.218, 489.601, 2.7864, 6423.855, 416.598, 508.305},
}

local timers = {}
local bannerSummons = {} -- per-guid latched EVENT_SPAWN summons (C++ JustSummoned :450-452 latch; drained by JustDied :457)

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

local function despawnBannerSummons(guid)
    local list = bannerSummons[guid]
    if list then
        for _, summon in ipairs(list) do
            if summon then
                summon:Despawn()
            end
        end
        bannerSummons[guid] = nil
    end
end

-- C++ EVENT_SPAWN (zone_icecrown.cpp:465-514): 7 DoSummon (default
-- 30s CORPSE_TIMED_DESPAWN = Lua type 5 + 30000ms, slad'ran precedent)
-- + MovePoint(0, walk-in) each; no DoZoneInCombat/AttackStart in C++.
local function spawnBannerCrew(creature, guid)
    if not creature:IsAlive() then
        return
    end
    local list = {}
    bannerSummons[guid] = list
    for _, s in ipairs(spawnTable) do
        local summon = creature:SummonCreature(s[1], s[2], s[3], s[4], s[5], 5, 30000)
        if summon then
            summon:MoveTo(0, s[6], s[7], s[8])
            list[#list + 1] = summon
        end
    end
end

-- C++ Reset() arm in arm order: SetRegenerateHealth(false) (bridged —
-- lua_creature_events.go, honored by the Go regen tick) -> non-triggered
-- self-cast SPELL_THREAT_PULSE + Talk(BANNER_SAY) -> ScheduleEvent
-- (EVENT_SPAWN, 3s). The banner never engages, so the fragment is
-- one-shot per spawn / reset; the 3s EVENT_SPAWN crew (7 DoSummon +
-- MovePoint walk-ins, zone_icecrown.cpp:465-514) ports via the MoveTo
-- twin with return-value latching (coren precedent); per-NPC GUID vars
-- (guidDalfors/guidPriest/guidMason) stay out — write-only until the
-- intro/wave legs port (zuramat _voidDance precedent).
local function onSpawnOrReset(_, creature)
    -- C++ npc_blessed_bannerAI::Reset(): SetRegenerateHealth(false) first,
    -- unconditionally (zone_icecrown.cpp:437-443), then the alive-gated
    -- cast + talk.
    creature:SetRegenerateHealth(false)
    -- C++ npc_blessed_bannerAI::Reset(): DoCast(SPELL_THREAT_PULSE) +
    -- Talk(BANNER_SAY) (zone_icecrown.cpp). Alive-gated so the death-leg
    -- ev23 fire (banner self-despawns in JustDied) is not phantom; the
    -- 3s EVENT_SPAWN crew ported above (MoveTo twin, coren latch).
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PULSE)
        creature:Talk(BANNER_SAY)
        -- C++ Reset: events.ScheduleEvent(EVENT_SPAWN, 3s) (:442).
        local guid = creature:GetGUID()
        cancelTimers(guid)
        schedule(guid, "spawn", 3000, function()
            spawnBannerCrew(creature, guid)
        end)
    end
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 5, onSpawnOrReset)
RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 23, onSpawnOrReset)

-- C++ npc_blessed_bannerAI::JustDied (zone_icecrown.cpp:455-459):
-- Summons.DespawnAll() ports via the EVENT_SPAWN latch (despawn before
-- self-Despawn, C++ order); me->DespawnOrUnsummon() ports
-- (creature:Despawn, bridged).
local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustDied order (:455-459): Summons.DespawnAll() first, then
    -- me->DespawnOrUnsummon().
    despawnBannerSummons(guid)
    creature:Despawn()
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 4, onDied)

-- Leave combat (event 2): no C++ evade drain for the banner (Reset
-- re-schedules EVENT_SPAWN on ev23); cancel the pending crew only.
local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 2, onLeaveCombat)

-- C++ npc_blessed_bannerAI::MoveInLineOfSight (zone_icecrown.cpp:447) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the banner is a quest object, never attacks). Go's
-- fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :447 C++-exact (the cyanigosa precedent).
local function onMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 27, onMoveInLOS)
RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- zone_icecrown.cpp:461: custom UpdateAI never calls DoMeleeAttackIfReady (summon event machine in C++, never melees)
    creature:SetCombatMovement(false) -- zone_icecrown.cpp:421: ctor arm, ScriptedAI stands-and-casts (eredar_twins convention)
end)
