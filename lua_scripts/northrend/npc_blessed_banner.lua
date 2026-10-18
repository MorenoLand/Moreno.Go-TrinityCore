-- Blessed Banner (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_blessed_bannerAI — Reset() arm only). Zone-script unit
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
-- mushroom precedent); OnDied(4) carries no modeled arm (no
-- timers to cancel — the EVENT_ENDED / JustDied despawn chain
-- sits behind the summon / despawn bridges, unmodeled).
-- Unmodeled: SetRegenerateHealth(false) (no regen bridge);
-- EVENT_SPAWN 3s (DoSummon 31003 + 3x 30919 + 3x 30900 with
-- MovePoint machines — summon / movement bridges);
-- EVENT_INTRO_1..3 Talk/facing chain (cross-creature-Talk /
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
-- SetHomePosition + EnterEvadeMode) has no DamageTaken hook /
-- player-self-cast / faction / despawn bridges; the ctor
-- MovePoint + SetFaction(FACTION_FRIENDLY) arm and the
-- MovementInform POINT_MOTION_TYPE -> SetFaction(FACTION_
-- MONSTER) flip sit behind the movement / faction bridges —
-- documented-only. 63010/65147/63049 verifiable.
-- npc_guardian_pavilion — the trespasser machine is entirely
-- MoveInLineOfSight-driven (no proximity hook — Smeed /
-- mageguard_dalaran precedent): GetAreaId() != 4676 (Sunreaver)
-- && != 4677 (Silver Covenant) gate (no area-ID bridge —
-- talbot precedent), IsHostileTo + isInBackInMap(who, 5.0f)
-- gates (no proximity / facing bridges), HasAura(63987/63986)
-- trespasser gates (player:HasAura is bridged — lua_player.go:203;
-- still blocked by the proximity / area-ID / facing / team /
-- player-self-cast bridges),
-- GetTeamId() == TEAM_ALLIANCE branch (no team bridge —
-- dalaran precedent), who->CastSpell(who, 63987/63986, true)
-- self-cast on the player (no player-self-cast path from a
-- creature AI — corastrasza precedent) — documented-only. No
-- NPC_ entry constant in the C++ sources; 63987/63986/4676/
-- 4677 verifiable.
-- npc_tournament_training_dummy — SpellHit credit machines PORTED
-- 2026-10-18 (see the port block below; event 14 is live —
-- volkhan precedent — retracted the stale "no SpellHit bridge"
-- claim). Remaining documented-only: DamageTaken -> damage = 0 +
-- EVENT_DUMMY_RESET reschedule 10s (no DamageTaken hook in the
-- model — lecraft events-9/14 precedent); Reset ->
-- EVENT_DUMMY_RECAST_DEFEND 5s (DoCast(me, 64100) / stack-gated
-- DoCast(me, 62719) — the 62719 leg needs GetStackAmount/
-- GetDuration, unbridged, so a blind 5s recast would
-- double-apply — not C++-exact — borean
-- imprisoned_beryl_sorcerer precedent); the stunned control
-- (SetControlled UNIT_STATE_STUNNED — no unit-state bridge —
-- twilight_corrupter precedent); MoveInLineOfSight overridden
-- empty; 62709 counterattack on the vehicle's base (no
-- vehicle-base bridge). 33272/33229/33243 + 62658/62672/62673
-- + 62544/62626/62874 + 62719/64100/62665/62709 verifiable.
-- npc_frostbrood_skytalon (VehicleAI) — the UpdateAI low-hp /
-- periodic arms run through VehicleAI::UpdateAI (the base is
-- unmodeled — wyrmrest_defender precedent); SpellHit 59335 ->
-- self-cast 54690 / 59319 -> DoCastAOE(59375) + EVENT_FLY_AWAY
-- 100ms (no SpellHit bridge — fizzule precedent); IsSummonedBy
-- MovePoint(POINT_GRAB_DECOY) + MovementInform -> DoCast(
-- summoner, 59318) behind the movement / summon bridges —
-- documented-only. No NPC_ entry constant in the C++ sources;
-- 59318/59375/54690/59335/59319 verifiable.

local SPELL_THREAT_PULSE = 58113

local BANNER_SAY = 0

local BLESSED_BANNER_ENTRY = 30891

-- C++ Reset() arm in arm order: SetRegenerateHealth(false)
-- unmodeled (no regen bridge) -> non-triggered self-cast
-- SPELL_THREAT_PULSE + Talk(BANNER_SAY). The banner never
-- engages, so the fragment is one-shot per spawn / reset;
-- no timers, no per-GUID state.
local function onSpawnOrReset(_, creature)
    -- C++ npc_blessed_bannerAI::Reset(): DoCast(SPELL_THREAT_PULSE) +
    -- Talk(BANNER_SAY) (zone_icecrown.cpp). Alive-gated so the death-leg
    -- ev23 fire (banner self-despawns in JustDied) is not phantom; the
    -- 3s EVENT_SPAWN summon leg is unported (no summon bridge).
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PULSE)
        creature:Talk(BANNER_SAY)
    end
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 5, onSpawnOrReset)
RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 23, onSpawnOrReset)

-- C++ npc_tournament_training_dummy::SpellHit (zone_icecrown.cpp:237-267)
-- credit machines (PORTED 2026-10-18, sweep6 candidate 2; the stale
-- "no SpellHit bridge" claim is retracted - event 14 is live, volkhan
-- precedent; the 62544->62658 / 62874->62658 header mis-pairing is
-- fixed: C++ has 62544->62672 MELEE_CREDIT, 62874->62658 CHARGE_CREDIT).
-- Per-GUID isVulnerable latch, reset on spawn/reset (C++ Initialize).
-- The 62709 counterattack-on-vehicle-base leg stays out (no
-- vehicle-base bridge); the DamageTaken / recast-defend / stunned-
-- control legs stay documented in the header.
local SPELL_CHARGE_CREDIT = 62658
local SPELL_MELEE_CREDIT = 62672
local SPELL_RANGED_CREDIT = 62673
local SPELL_PLAYER_THRUST = 62544
local SPELL_PLAYER_BREAK_SHIELD = 62626
local SPELL_PLAYER_CHARGE = 62874
local SPELL_RANGED_DEFEND = 62719
local SPELL_CHARGE_DEFEND = 64100

local NPC_CHARGE_TARGET = 33272
local NPC_MELEE_TARGET = 33229
local NPC_RANGED_TARGET = 33243

local dummyVulnerable = {}

local function onDummyReset(_, creature)
    dummyVulnerable[creature:GetGUID()] = nil
end

local function onDummySpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUID()
    local entry = creature:GetEntry()
    if entry == NPC_CHARGE_TARGET then
        if spellId == SPELL_PLAYER_CHARGE and dummyVulnerable[guid] then
            creature:CastSpell(caster, SPELL_CHARGE_CREDIT, true)
        end
    elseif entry == NPC_MELEE_TARGET then
        if spellId == SPELL_PLAYER_THRUST then
            creature:CastSpell(caster, SPELL_MELEE_CREDIT, true)
        end
    elseif entry == NPC_RANGED_TARGET then
        if spellId == SPELL_PLAYER_BREAK_SHIELD and dummyVulnerable[guid] then
            creature:CastSpell(caster, SPELL_RANGED_CREDIT, true)
        end
    end
    if spellId == SPELL_PLAYER_BREAK_SHIELD then
        if not creature:HasAura(SPELL_CHARGE_DEFEND)
                and not creature:HasAura(SPELL_RANGED_DEFEND) then
            dummyVulnerable[guid] = true
        end
    end
end

RegisterCreatureEvent(NPC_CHARGE_TARGET, 14, onDummySpellHit)
RegisterCreatureEvent(NPC_MELEE_TARGET, 14, onDummySpellHit)
RegisterCreatureEvent(NPC_RANGED_TARGET, 14, onDummySpellHit)
RegisterCreatureEvent(NPC_CHARGE_TARGET, 5, onDummyReset)
RegisterCreatureEvent(NPC_MELEE_TARGET, 5, onDummyReset)
RegisterCreatureEvent(NPC_RANGED_TARGET, 5, onDummyReset)
RegisterCreatureEvent(NPC_CHARGE_TARGET, 23, onDummyReset)
RegisterCreatureEvent(NPC_MELEE_TARGET, 23, onDummyReset)
RegisterCreatureEvent(NPC_RANGED_TARGET, 23, onDummyReset)
