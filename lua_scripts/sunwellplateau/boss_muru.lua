-- Muru / Entropius (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_muru.cpp
-- (boss_muru, boss_entropius, npc_muru_portal, npc_dark_fiend,
-- npc_void_sentinel, npc_blackhole; the spell_summon_blood_elves_
-- script / spell_muru_darkness SpellScripts and the spell_dark_
-- fiend_skin / spell_transform_visual_missile_periodic / spell_
-- summon_blood_elves_periodic AuraScripts documented below, not
-- registered); sunwell_plateau.h:35 (DATA_MURU = 4, fifth boss),
-- :60 (NPC_MURU = 25741), :61 (NPC_ENTROPIUS = 25840), :96
-- (NPC_DARKNESS = 25879), :97 (NPC_DARK_FIENDS = 25744), :100
-- (NPC_VOID_SENTINEL = 25772), :103 (NPC_MURU_PORTAL_TARGET =
-- 25770). Entries: 25741 Muru (C++ ScriptName "boss_muru"), 25840
-- Entropius ("boss_entropius"), 25770 Muru Portal Target ("npc_
-- muru_portal"), 25744 Dark Fiend ("npc_dark_fiend"), 25772 Void
-- Sentinel ("npc_void_sentinel"), 25879 Darkness ("npc_blackhole")
-- — all entry+ScriptName verifiable from the C++ sources per
-- RegisterSunwellPlateauCreatureAI in AddSC_boss_muru; the
-- creature_template ScriptName bindings are DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified. The
-- C++ file has no Talk calls at all — no creature_text lines are
-- used in this fight. Eluna creature events: 1 OnEnterCombat, 2
-- OnLeaveCombat, 4 OnDied, 9 OnDamageTaken (returns false,
-- damage), 14 OnSpellHit, 23 OnReset. Timers via CreateLuaEvent.
-- Melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady — except Muru itself, whose C++ UpdateAI
-- only pumps the scheduler (no DoMeleeAttackIfReady arm), so no
-- melee note applies to 25741.
-- Fight shape (C++-exact for the modeled arms): Muru (25741).
-- OnEnterCombat(1): per-GUID reset (phase=1, hasEnraged=false),
-- triggered self-cast open portal periodic 45994 + darkness
-- periodic 45998 + negative energy periodic 46009 (C++ DoCast(me,
-- X, true), C++-exact — the periodic summon arms live in the
-- unmodeled spell_summon_blood_elves_periodic / spell_muru_
-- darkness scripts, below); 10s one-shot: triggered self-cast
-- summon blood elves script 46050 + summon blood elves periodic
-- 46041 (C++ ScheduleTasks, C++-exact — the actual blood-elf
-- summons are summon-blocked, below); enrage 26662 10min one-shot:
-- non-triggered self-cast (C++ DoCast default is triggered=false
-- — vaelastrasz convention), hasEnraged=true (the entropius
-- cross-creature cast has no cross-creature bridge — below).
-- OnDamageTaken(9, pre-damage hook): lethal from any source ->
-- damage rewritten to health-1 (C++-exact); additionally, in
-- PHASE_ONE only: phase=PHASE_TWO, triggered self-cast open all
-- portals 46177, then 6s later triggered self-cast summon
-- entropius 46217 (C++-exact ordering; the RemoveAllAuras and
-- NOT_SELECTABLE-flag arms have no bridges — below). OnDied(4):
-- timer/state cleanup only (the _JustDied arm is blocked on the
-- instance-script model). OnReset(23)/OnLeaveCombat(2): timer/
-- state cleanup (the RemoveFlag(NOT_SELECTABLE)/SetVisible(true)
-- arms have no flag/visibility bridges; the _Reset instance
-- bookkeeping and the EnterEvadeMode entropius relay are blocked
-- on the instance-script/cross-creature bridges). The
-- SetCombatMovement(false) constructor arm has no movement
-- bridge — Muru never moves in this model (C++-exact, since it
-- never schedules movement). Entropius (25840). OnReset(23):
-- triggered self-cast entropius cosmetic spawn 46223 (C++-exact;
-- the _Reset instance bookkeeping is blocked). OnEnterCombat(1):
-- 2s one-shot: triggered self-cast (DoCastAOE self-position,
-- kalecgos convention) negative energy periodic E 46284 (the
-- DoResetPortals 100yd grid scan of entry 25770 -> RemoveAllAuras
-- has no creature-enumeration bridge — skipped); 15s repeat:
-- triggered self-cast darkness E 46269 + triggered self-cast
-- blackhole 46282 (the dark-fiend summon arm of 46269 lives in
-- the unmodeled spell_muru_darkness SpellScript — below).
-- UpdateAI melee is engine-driven (C++ DoMeleeAttackIfReady).
-- OnDied(4): timer cleanup only (the _JustDied + muru->
-- DisappearAndDie arms are blocked on the instance-script/
-- cross-creature/kill bridges). OnReset(23)/OnLeaveCombat(2):
-- timer cleanup (the EnterEvadeMode muru relay, DoResetPortals,
-- summons.DespawnAll and DespawnOrUnsummon arms have no cross-
-- creature/enumeration/summon/despawn bridges — below).
-- The entropius JustSummoned arms (NPC_DARK_FIENDS -> visual
-- 45936; NPC_DARKNESS -> REACT_PASSIVE + 46282 self-cast +
-- triggered 46263 summon-darkfiend) are summon-blocked —
-- unmodeled. Portal (25770). OnSpellHit(14): hit by OPEN_ALL_
-- PORTALS 46177 -> triggered DoCastAOE(OPEN_PORTAL 45977) +
-- triggered DoCastAOE(TRANSFORM_VISUAL_MISSILE 46205) (C++-
-- exact); hit by OPEN_PORTAL_2 45976 -> triggered DoCastAOE(
-- 45977) + 6s later triggered DoCastAOE(SUMMON_VOID_SENTINEL_
-- SUMMONER 45978) (C++-exact). Whether 46177/45976 reach the
-- portals in the Go model depends on the engine's hit resolution
-- for those casts — the AI arms are registered faithfully (the
-- kalecgos teleport-back event-14 precedent). The JustSummoned
-- arm (triggered 45989 summoner visual + 1.5s-delayed self-cast
-- 45988 summon-void-sentinel, driving the void-sentinel chain)
-- is summon-blocked — unmodeled. Dark Fiend (25744). OnReset(23):
-- triggered self-cast darkfiend skin 45934 (C++-exact — the
-- constructor/Initialize arm; the SetDisplayId/REACT_PASSIVE arms
-- have no display/react bridges). The 2s arm (REACT_AGGRESSIVE
-- + RemoveFlag(NOT_SELECTABLE) + summoner-GUID random-target
-- AttackStart) has no react/flag/cross-creature/movement bridges
-- — skipped; the 3s 500ms-repeat proximity arm (victim within
-- 5yd + HasAura(45934) -> non-triggered DoCastAOE(DARKFIEND_
-- DAMAGE 45944) + DisappearAndDie) is unmodeled as a whole — the
-- kill/despawn arm has no bridge and a cast without the despawn
-- would not be C++-exact. The IsSummonedBy summoner-GUID capture
-- is summon-blocked; the CanAIAttack aura gate is engine-side.
-- Void Sentinel (25772). OnEnterCombat(1): triggered self-cast
-- shadow pulse periodic 46086 (C++ DoCast(me, ..., true),
-- C++-exact); void blast 46161 45s repeat, non-triggered
-- DoCastVictim (nil ticks cast nothing but keep the schedule,
-- jeklik convention). The IsSummonedBy muru-relay arm is summon/
-- instance blocked; the JustDied 6x triggered DoCastAOE(SUMMON_
-- VOID_SPAWN 46071) arm is summon-blocked — skipped. UpdateAI
-- melee is engine-driven (C++ DoMeleeAttackIfReady). Darkness /
-- Black Hole (25879). OnReset(23): non-triggered self-cast
-- blackhole summon visual 46242 (C++ DoCast default is
-- triggered=false — vaelastrasz convention; the REACT_PASSIVE
-- arm has no react bridge), then the C++ repeat-counter visual
-- chain: after 1s cast 46247 (the REACT_AGGRESSIVE + AttackStart
-- (DATA_PLAYER_GUID) arms have no react/instance-player/movement
-- bridges — skipped), after a further 1.2s cast 46242, after a
-- further 2s cast 46228 + 46235, then the chain ends (C++-exact
-- — no repeat after the counter-2 arm). The 15s
-- DisappearAndDie arm has no despawn bridge — skipped.
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (BossAI::
-- JustEngagedWith/JustDied/Reset and the DATA_MURU bookkeeping
-- arms skipped); no cross-creature bridge — the muru/enrage,
-- entropius-evade and entropius-death relays to DATA_MURU and
-- the dark-fiend summoner-GUID target pick are unmodeled; no
-- summon bridge — the blood-elf/void-spawn/void-sentinel/
-- dark-fiend/entropius spawns and the entropius JustSummoned
-- visual/passive arms unmodeled; no SpellScript/AuraScript
-- bridges — spell_summon_blood_elves_script (4 random 46037/
-- 46040/46038/46039 casts), spell_muru_darkness (8 dark-fiend
-- summon casts 46000-46007 after cast), spell_dark_fiend_skin
-- (dispel-remove -> passive + 45936 visual + 3s despawn),
-- spell_transform_visual_missile_periodic (periodic 46208/46178
-- missile) and spell_summon_blood_elves_periodic (periodic 46050
-- re-cast) unmodeled (standing gaps); no aura-enumeration/
-- flag/visibility/react/movement/kill/despawn bridges — the
-- muru phase-two RemoveAllAuras + NOT_SELECTABLE arms, the
-- portal DoResetPortals grid scan, the dark-fiend display/react/
-- proximity-despawn arms, the black-hole 15s despawn and the
-- entropius evade despawn-all arms unmodeled.

local SPELL_OPEN_PORTAL_PERIODIC = 45994
local SPELL_DARKNESS_PERIODIC = 45998
local SPELL_NEGATIVE_ENERGY_PERIODIC = 46009
local SPELL_SUMMON_BLOOD_ELVES_SCRIPT = 46050
local SPELL_SUMMON_BLOOD_ELVES_PERIODIC = 46041
local SPELL_OPEN_ALL_PORTALS = 46177
local SPELL_SUMMON_ENTROPIUS = 46217
local SPELL_ENRAGE = 26662
local SPELL_ENTROPIUS_COSMETIC_SPAWN = 46223
local SPELL_NEGATIVE_ENERGY_PERIODIC_E = 46284
local SPELL_DARKNESS_E = 46269
local SPELL_BLACKHOLE = 46282
local SPELL_OPEN_PORTAL = 45977
local SPELL_OPEN_PORTAL_2 = 45976
local SPELL_SUMMON_VOID_SENTINEL_SUMMONER = 45978
local SPELL_TRANSFORM_VISUAL_MISSILE = 46205
local SPELL_DARKFIEND_SKIN = 45934
local SPELL_SHADOW_PULSE_PERIODIC = 46086
local SPELL_VOID_BLAST = 46161
local SPELL_BLACKHOLE_SUMMON_VISUAL = 46242
local SPELL_BLACKHOLE_SUMMON_VISUAL_2 = 46247
local SPELL_BLACKHOLE_PASSIVE = 46228
local SPELL_BLACK_HOLE_VISUAL_2 = 46235

local ENTRY_MURU = 25741
local ENTRY_ENTROPIUS = 25840
local ENTRY_PORTAL = 25770
local ENTRY_DARK_FIEND = 25744
local ENTRY_VOID_SENTINEL = 25772
local ENTRY_BLACKHOLE = 25879

local timers = {}
local muruState = {}

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

-- ================= Muru (25741) =================

-- C++ JustEngagedWith: triggered self-casts 45994/45998/46009;
-- 10s one-shot triggered 46050 + 46041; 10min one-shot
-- non-triggered enrage 26662.
local function muruEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    muruState[guid] = { phase = 1, hasEnraged = false }
    creature:CastSpell(creature, SPELL_OPEN_PORTAL_PERIODIC, true)
    creature:CastSpell(creature, SPELL_DARKNESS_PERIODIC, true)
    creature:CastSpell(creature, SPELL_NEGATIVE_ENERGY_PERIODIC, true)
    schedule(guid, "bloodelves", 10000, function()
        creature:CastSpell(creature, SPELL_SUMMON_BLOOD_ELVES_SCRIPT, true)
        creature:CastSpell(creature, SPELL_SUMMON_BLOOD_ELVES_PERIODIC, true)
    end)
    schedule(guid, "enrage", 600000, function()
        creature:CastSpell(creature, SPELL_ENRAGE)
        local st = muruState[guid]
        if st then
            st.hasEnraged = true
        end
    end)
end

-- C++ DamageTaken: lethal -> damage = health - 1; PHASE_ONE only:
-- phase=PHASE_TWO, triggered 46177, 6s-delayed triggered 46217.
local function muruDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health then
        local guid = creature:GetGUID()
        local st = muruState[guid]
        if st and st.phase == 1 then
            st.phase = 2
            creature:CastSpell(creature, SPELL_OPEN_ALL_PORTALS, true)
            schedule(guid, "summonentropius", 6000, function()
                creature:CastSpell(creature, SPELL_SUMMON_ENTROPIUS, true)
            end)
        end
        return false, health - 1
    end
end

local function muruResetState(guid)
    cancelTimers(guid)
    muruState[guid] = nil
end

local function muruLeaveCombat(event, creature)
    muruResetState(creature:GetGUID())
end

local function muruDied(event, creature, killer)
    muruResetState(creature:GetGUID())
end

local function muruReset(event, creature)
    muruResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MURU, 1, muruEnterCombat)
RegisterCreatureEvent(ENTRY_MURU, 2, muruLeaveCombat)
RegisterCreatureEvent(ENTRY_MURU, 4, muruDied)
RegisterCreatureEvent(ENTRY_MURU, 9, muruDamageTaken)
RegisterCreatureEvent(ENTRY_MURU, 23, muruReset)

-- ================= Entropius (25840) =================

-- C++ ScheduleTasks: 2s one-shot triggered 46284; 15s repeat
-- triggered 46269 + triggered 46282.
local function entropiusNegativeEnergy(creature, guid)
    creature:CastSpell(creature, SPELL_NEGATIVE_ENERGY_PERIODIC_E, true)
end

local function entropiusDarkness(creature, guid)
    creature:CastSpell(creature, SPELL_DARKNESS_E, true)
    creature:CastSpell(creature, SPELL_BLACKHOLE, true)
    schedule(guid, "darkness", 15000, function()
        entropiusDarkness(creature, guid)
    end)
end

local function entropiusEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "negativeenergy", 2000, function()
        entropiusNegativeEnergy(creature, guid)
    end)
    schedule(guid, "darkness", 15000, function()
        entropiusDarkness(creature, guid)
    end)
end

local function entropiusCleanup(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: triggered self-cast 46223 (constructor/cosmetic arm).
local function entropiusReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_ENTROPIUS_COSMETIC_SPAWN, true)
end

RegisterCreatureEvent(ENTRY_ENTROPIUS, 1, entropiusEnterCombat)
RegisterCreatureEvent(ENTRY_ENTROPIUS, 2, entropiusCleanup)
RegisterCreatureEvent(ENTRY_ENTROPIUS, 4, entropiusCleanup)
RegisterCreatureEvent(ENTRY_ENTROPIUS, 23, entropiusReset)

-- ================= Muru Portal Target (25770) =================

-- C++ SpellHit: 46177 -> triggered 45977 + triggered 46205;
-- 45976 -> triggered 45977 + 6s-delayed triggered 45978.
local function portalSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_OPEN_ALL_PORTALS then
        creature:CastSpell(creature, SPELL_OPEN_PORTAL, true)
        creature:CastSpell(creature, SPELL_TRANSFORM_VISUAL_MISSILE, true)
    elseif spellId == SPELL_OPEN_PORTAL_2 then
        creature:CastSpell(creature, SPELL_OPEN_PORTAL, true)
        local guid = creature:GetGUID()
        schedule(guid, "voidsentinelsummoner", 6000, function()
            creature:CastSpell(creature, SPELL_SUMMON_VOID_SENTINEL_SUMMONER, true)
        end)
    end
end

local function portalReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PORTAL, 14, portalSpellHit)
RegisterCreatureEvent(ENTRY_PORTAL, 23, portalReset)

-- ================= Dark Fiend (25744) =================

-- C++ constructor (Initialize): triggered self-cast 45934.
local function darkFiendReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_DARKFIEND_SKIN, true)
end

RegisterCreatureEvent(ENTRY_DARK_FIEND, 23, darkFiendReset)

-- ================= Void Sentinel (25772) =================

-- C++ JustEngagedWith: triggered self-cast 46086; 45s repeat
-- non-triggered DoCastVictim(46161).
local function voidSentinelVoidBlast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_VOID_BLAST)
    end
    schedule(guid, "voidblast", 45000, function()
        voidSentinelVoidBlast(creature, guid)
    end)
end

local function voidSentinelEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_SHADOW_PULSE_PERIODIC, true)
    schedule(guid, "voidblast", 45000, function()
        voidSentinelVoidBlast(creature, guid)
    end)
end

local function voidSentinelCleanup(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_VOID_SENTINEL, 1, voidSentinelEnterCombat)
RegisterCreatureEvent(ENTRY_VOID_SENTINEL, 2, voidSentinelCleanup)
RegisterCreatureEvent(ENTRY_VOID_SENTINEL, 23, voidSentinelCleanup)

-- ================= Darkness / Black Hole (25879) =================

-- C++ Reset: non-triggered self-cast 46242, then the repeat-
-- counter visual chain: 1s -> 46247; 1.2s -> 46242; 2s -> 46228
-- + 46235, then the chain ends.
local function blackholeVisualStep2(creature, guid)
    creature:CastSpell(creature, SPELL_BLACKHOLE_PASSIVE)
    creature:CastSpell(creature, SPELL_BLACK_HOLE_VISUAL_2)
end

local function blackholeVisualStep1(creature, guid)
    creature:CastSpell(creature, SPELL_BLACKHOLE_SUMMON_VISUAL)
    schedule(guid, "visual", 2000, function()
        blackholeVisualStep2(creature, guid)
    end)
end

local function blackholeVisualStep0(creature, guid)
    creature:CastSpell(creature, SPELL_BLACKHOLE_SUMMON_VISUAL_2)
    schedule(guid, "visual", 1200, function()
        blackholeVisualStep1(creature, guid)
    end)
end

local function blackholeReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_BLACKHOLE_SUMMON_VISUAL)
    schedule(guid, "visual", 1000, function()
        blackholeVisualStep0(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_BLACKHOLE, 23, blackholeReset)
