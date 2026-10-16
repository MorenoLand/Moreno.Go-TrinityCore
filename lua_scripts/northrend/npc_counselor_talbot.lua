-- Counselor Talbot (Borean Tundra, Last Rites quest chain) --
-- Lua port of npc_counselor_talbotAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 832-945;
-- AddSC at line 1888, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_COUNSELOR_TALBOT = 25301 (:413). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Talbot is summoned
-- at fixed coords (3747.23, 3614.936, 473.321) by npc_thassarian
-- (:513) and despawns with the quest, so he never fights outside his
-- staging area.
-- Zone-script unit per northrend_script_loader.cpp order (borean_tundra:
-- 20 registrations; cultist decl 199 / call 394).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize() (:844-851) timer seeds — SHADOWBOLT
--   urand(5000,12000), DEFLECTION urand(20000,25000), SOULBLAST
--   urand(12000,18000). C++ decrements these only inside the
--   UpdateVictim-gated UpdateAI (:878-907), so seeding on combat start
--   is the faithful Reset-seed lowering (svalna precedent).
-- - EVENT_SHADOWBOLT: non-triggered DoCastVictim(SPELL_SHADOW_BOLT
--   15537); re-arm urand(5000,12000).
-- - EVENT_DEFLECTION: non-triggered DoCastVictim(SPELL_DEFLECTION
--   51009); re-arm urand(20000,25000).
-- - EVENT_SOULBLAST: non-triggered DoCastVictim(SPELL_SOUL_BLAST
--   50992); re-arm urand(12000,18000).
-- - OnLeaveCombat / OnDied / OnReset: cancel GUID-keyed timers
--   (C++ events.Reset() residue).
-- Unmodeled (documented-only, no bridges):
-- - UpdateAI's `if (me->GetAreaId() == 4125)` gate (:889): no GetAreaId
--   bridge. Talbot is a TEMPSUMMON_CORPSE_TIMED_DESPAWN at fixed quest
--   coords (:513) — he cannot be in combat outside the staging area,
--   so the missing gate has no behavioral delta in practice.
-- - bCheck / FindNearestCreature(NPC_LERYSSA 25251, NPC_GENERAL_ARLOS
--   25250, 50.0f) GUID latch (:876-886): no FindNearestCreature /
--   ObjectAccessor bridge (dark-rider precedent).
-- - MovementInform(POINT_MOTION_TYPE) -> ENSURE_AI(npc_thassarian)
--   talbotInPosition cross-AI latch (:860-868): no MovementInform /
--   cross-AI-ENSURE bridge.
-- - JustDied (:913-938): arlos->AI()->Talk / arlos->KillSelf /
--   leryssa->RemoveAura / ClearUnitState / SetWalk / MovePoint /
--   RewardPlayerAndGroupAtEvent — no ObjectAccessor, motion,
--   reward, or unit-state bridges (kalecgos / death_knight_initiate
--   precedents).
-- Verifiable numbers (file's own enums): SPELL_SHADOW_BOLT = 15537
-- (:410), SPELL_DEFLECTION = 51009 (:827), SPELL_SOUL_BLAST = 50992
-- (:828), NPC_COUNSELOR_TALBOT = 25301 (:413).

local ENTRY_COUNSELOR_TALBOT = 25301

local SPELL_SHADOW_BOLT = 15537
local SPELL_DEFLECTION = 51009
local SPELL_SOUL_BLAST = 50992

local timers = {}

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

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ ShadowBolt: DoCastVictim(SPELL_SHADOW_BOLT 15537);
-- urand(5000,12000) re-arm.
local function onShadowBolt(creature, guid)
    doCastVictim(creature, SPELL_SHADOW_BOLT)
    schedule(guid, "shadow_bolt", math.random(5000, 12000), function() onShadowBolt(creature, guid) end)
end

-- C++ Deflection: DoCastVictim(SPELL_DEFLECTION 51009);
-- urand(20000,25000) re-arm.
local function onDeflection(creature, guid)
    doCastVictim(creature, SPELL_DEFLECTION)
    schedule(guid, "deflection", math.random(20000, 25000), function() onDeflection(creature, guid) end)
end

-- C++ SoulBlast: DoCastVictim(SPELL_SOUL_BLAST 50992);
-- urand(12000,18000) re-arm.
local function onSoulBlast(creature, guid)
    doCastVictim(creature, SPELL_SOUL_BLAST)
    schedule(guid, "soul_blast", math.random(12000, 18000), function() onSoulBlast(creature, guid) end)
end

-- C++ Initialize()/Reset(): SHADOWBOLT urand(5s,12s), DEFLECTION
-- urand(20s,25s), SOULBLAST urand(12s,18s). Timers only tick
-- UpdateVictim-gated in C++, so seeding at combat start is faithful.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadow_bolt", math.random(5000, 12000), function() onShadowBolt(creature, guid) end)
    schedule(guid, "deflection", math.random(20000, 25000), function() onDeflection(creature, guid) end)
    schedule(guid, "soul_blast", math.random(12000, 18000), function() onSoulBlast(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 4, onDied)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 23, onReset)
