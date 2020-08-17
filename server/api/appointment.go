package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi"
	"github.com/segmentio/ksuid"
)

const (
	appointmentDayContextKey      = "appointment_day"
	appointmentTimeslotContextKey = "appointment_timeslot"
	appointmentContextKey         = "appointment"
)

func (s *Server) AppointmentDayMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		day, err := strconv.Atoi(chi.URLParam(r, "day"))
		if err != nil {
			s.logger.Warnw("failed to parse day",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"day", chi.URLParam(r, "day"),
				"params", chi.RouteContext(r.Context()).URLParams,
				"err", err,
			)
			s.errorMessage(
				http.StatusNotFound,
				fmt.Sprintf(`Invalid day "%s"`, chi.URLParam(r, "day")),
				w, r,
			)
			return
		}

		ctx := context.WithValue(r.Context(), appointmentDayContextKey, day)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) AppointmentTimeslotMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeslot, err := strconv.Atoi(chi.URLParam(r, "timeslot"))
		if err != nil {
			s.logger.Warnw("failed to parse timeslot",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"timeslot", chi.URLParam(r, "timeslot"),
				"params", chi.RouteContext(r.Context()).URLParams,
				"err", err,
			)
			s.errorMessage(
				http.StatusNotFound,
				fmt.Sprintf(`Invalid timeslot "%s"`, chi.URLParam(r, "timeslot")),
				w, r,
			)
			return
		}

		ctx := context.WithValue(r.Context(), appointmentTimeslotContextKey, timeslot)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type getAppointment interface {
	GetAppointment(ctx context.Context, appointment ksuid.KSUID) (*AppointmentSlot, error)
}

func (s *Server) AppointmentIDMiddleware(ga getAppointment) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "appointment_id")

			appointmentID, err := ksuid.Parse(id)
			if err != nil {
				s.logger.Warnw("failed to parse appointment ID",
					RequestIDContextKey, r.Context().Value(RequestIDContextKey),
					"appointment_id", id,
					"err", err,
				)
				s.errorMessage(
					http.StatusNotFound,
					"I called for help, but I couldn't find that appointment anywhere.",
					w, r,
				)
				return
			}

			appointment, err := ga.GetAppointment(r.Context(), appointmentID)
			if err != nil {
				s.logger.Warnw("failed to get non-existent appointment with valid ksuid",
					RequestIDContextKey, r.Context().Value(RequestIDContextKey),
					"appointment_id", id,
					"err", err,
				)
				s.errorMessage(
					http.StatusNotFound,
					"I called for help, but I couldn't find that appointment anywhere. Was it just deleted?",
					w, r,
				)
				return
			}

			ctx := context.WithValue(r.Context(), appointmentContextKey, appointment)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type getAppointmentsInTimeFrame interface {
	GetAppointments(ctx context.Context, queue ksuid.KSUID, from, to time.Time) ([]*AppointmentSlot, error)
}

type getAppointments interface {
	getAppointmentsInTimeFrame
	GetAppointmentsWithStudent(ctx context.Context, queue ksuid.KSUID, from, to time.Time) ([]*AppointmentSlot, error)
}

func (s *Server) GetAppointments(ga getAppointments) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		admin := r.Context().Value(queueAdminContextKey).(bool)
		day := r.Context().Value(appointmentDayContextKey).(int)

		var appointments []*AppointmentSlot
		var err error
		start, end := WeekdayBounds(day)
		if admin {
			appointments, err = ga.GetAppointments(r.Context(), q.ID, start, end)
		} else {
			appointments, err = ga.GetAppointmentsWithStudent(r.Context(), q.ID, start, end)
		}

		if err != nil {
			s.logger.Errorw("failed to get appointments",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"err", err,
			)
			s.internalServerError(w, r)
			return
		}

		s.sendResponse(http.StatusOK, appointments, w, r)
	}
}

type getAppointmentsForUser interface {
	GetAppointmentsForUser(ctx context.Context, queue ksuid.KSUID, from, to time.Time, email string) ([]*AppointmentSlot, error)
}

func (s *Server) GetAppointmentsForCurrentUser(ga getAppointmentsForUser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		email := r.Context().Value(emailContextKey).(string)
		day := r.Context().Value(appointmentDayContextKey).(int)

		start, end := WeekdayBounds(day)
		appointments, err := ga.GetAppointmentsForUser(r.Context(), q.ID, start, end, email)
		if err != nil {
			s.logger.Errorw("failed to get appointments for user",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"queue_id", q.ID,
				"email", email,
				"day", day,
			)
			s.internalServerError(w, r)
			return
		}

		s.sendResponse(http.StatusOK, appointments, w, r)
	}
}

type getAppointmentSchedule interface {
	GetAppointmentSchedule(ctx context.Context, queue ksuid.KSUID) ([]*AppointmentSchedule, error)
}

func (s *Server) GetAppointmentSchedule(gs getAppointmentSchedule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)

		schedules, err := gs.GetAppointmentSchedule(r.Context(), q.ID)
		if err != nil {
			s.logger.Errorw("failed to get appointment schedule",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"queue_id", q.ID,
				"err", err,
			)
			s.internalServerError(w, r)
			return
		}

		s.sendResponse(http.StatusOK, schedules, w, r)
	}
}

type getAppointmentScheduleForDay interface {
	GetAppointmentScheduleForDay(ctx context.Context, queue ksuid.KSUID, day int) (*AppointmentSchedule, error)
}

func (s *Server) GetAppointmentScheduleForDay(gs getAppointmentScheduleForDay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		day := r.Context().Value(appointmentDayContextKey).(int)

		schedule, err := gs.GetAppointmentScheduleForDay(r.Context(), q.ID, day)
		if err != nil {
			s.logger.Errorw("failed to get appointment schedule",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"queue_id", q.ID,
				"day", day,
				"err", err,
			)
			s.internalServerError(w, r)
			return
		}

		s.sendResponse(http.StatusOK, schedule, w, r)
	}
}

type claimTimeslot interface {
	ClaimTimeslot(ctx context.Context, queue ksuid.KSUID, day, timeslot int, email string) error
}

func (s *Server) ClaimTimeslot(cs claimTimeslot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		email := r.Context().Value(emailContextKey).(string)
		day := r.Context().Value(appointmentDayContextKey).(int)
		timeslot := r.Context().Value(appointmentTimeslotContextKey).(int)
		l := s.logger.With(
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"queue_id", q.ID,
			"day", day,
			"timeslot", timeslot,
			"email", email,
		)

		err := cs.ClaimTimeslot(r.Context(), q.ID, day, timeslot, email)
		if err != nil {
			l.Errorw("failed to claim timeslot", "err", err)
			s.errorMessage(
				http.StatusBadRequest,
				"Failed to claim timeslot. Perhaps it has already been claimed? error: "+err.Error(),
				w, r,
			)
			return
		}

		l.Infow("appointment claimed")
		s.sendResponse(http.StatusCreated, nil, w, r)
	}
}

type unclaimAppointment interface {
	UnclaimAppointment(ctx context.Context, appointment ksuid.KSUID) error
}

func (s *Server) UnclaimAppointment(us unclaimAppointment) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appointment := r.Context().Value(appointmentContextKey).(*AppointmentSlot)

		err := us.UnclaimAppointment(r.Context(), appointment.ID)
		if err != nil {
			s.logger.Errorw("failed to remove appointment claim",
				RequestIDContextKey, r.Context().Value(RequestIDContextKey),
				"appointment_id", appointment.ID,
				"err", err,
			)
			s.internalServerError(w, r)
			return
		}

		s.logger.Infow("removed appointment claim",
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"appointment_id", appointment.ID,
			"email", r.Context().Value(emailContextKey),
		)
		s.sendResponse(http.StatusNoContent, nil, w, r)
	}
}

type updateAppointmentSchedule interface {
	getAppointmentsInTimeFrame
	UpdateAppointmentSchedule(ctx context.Context, queue ksuid.KSUID, day int, schedule *AppointmentSchedule) error
}

func (s *Server) UpdateAppointmentSchedule(us updateAppointmentSchedule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		email := r.Context().Value(emailContextKey).(string)
		day := r.Context().Value(appointmentDayContextKey).(int)
		l := s.logger.With(
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"queue_id", q.ID,
			"day", day,
			"email", email,
		)

		from, to := WeekdayBounds(day)
		appointments, err := us.GetAppointments(r.Context(), q.ID, from, to)
		if err != nil {
			l.Errorw("failed to get appointments", "err", err)
			s.internalServerError(w, r)
			return
		}

		if len(appointments) > 0 {
			l.Warnw("appointment schedule update attempted with existing appointments")
			s.errorMessage(
				http.StatusConflict,
				"The schedule can't be changed with active appointments. Marty McFly…or something.",
				w, r,
			)
			return
		}

		var schedule AppointmentSchedule
		err = json.NewDecoder(r.Body).Decode(&schedule)
		if err != nil {
			l.Warnw("failed to decode schedule from body", "err", err)
			s.errorMessage(
				http.StatusBadRequest,
				"We couldn't read the schedule in the request body.",
				w, r,
			)
		}

		err = us.UpdateAppointmentSchedule(r.Context(), q.ID, day, &schedule)
		if err != nil {
			l.Errorw("failed to update appointment schedule", "err", err)
			s.internalServerError(w, r)
			return
		}

		l.Infow("updated appointment schedule")
		s.sendResponse(http.StatusNoContent, nil, w, r)
	}
}

type getAppointmentsByTimeslot interface {
	GetAppointmentsByTimeslot(ctx context.Context, queue ksuid.KSUID, from, to time.Time, timeslot int) ([]*AppointmentSlot, error)
}

type signupForAppointment interface {
	getAppointmentScheduleForDay
	getAppointmentsForUser
	getAppointmentsByTimeslot
	SignupForAppointment(ctx context.Context, queue ksuid.KSUID, appointment *AppointmentSlot) (*AppointmentSlot, error)
}

func (s *Server) SignupForAppointment(sa signupForAppointment) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.Context().Value(queueContextKey).(*Queue)
		day := r.Context().Value(appointmentDayContextKey).(int)
		timeslot := r.Context().Value(appointmentTimeslotContextKey).(int)
		email := r.Context().Value(emailContextKey).(string)
		l := s.logger.With(
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"queue_id", q.ID,
			"day", day,
			"timeslot", timeslot,
			"email", email,
		)

		var appointment AppointmentSlot
		err := json.NewDecoder(r.Body).Decode(&appointment)
		if err != nil {
			l.Warnw("failed to decode appointment", "err", err)
			s.errorMessage(
				http.StatusBadRequest,
				"We couldn't read your appointment in the request body.",
				w, r,
			)
			return
		}

		if appointment.Description == nil || appointment.Name == nil || appointment.Location == nil ||
			*appointment.Description == "" || *appointment.Name == "" || *appointment.Location == "" {
			l.Warnw("got incomplete appointment", "appointment", appointment)
			s.errorMessage(
				http.StatusBadRequest,
				"It looks like you left out some fields in the appointment.",
				w, r,
			)
			return
		}

		schedule, err := sa.GetAppointmentScheduleForDay(r.Context(), q.ID, day)
		if err != nil {
			l.Errorw("failed to get appointment schedule", "err", err)
			s.internalServerError(w, r)
			return
		}

		if timeslot > len(schedule.Schedule) {
			l.Warnw("attempted to sign up for non-existent timeslot", "num_slots", len(schedule.Schedule))
			s.errorMessage(
				http.StatusNotFound,
				"That timeslot doesn't exist!",
				w, r,
			)
			return
		}

		start, end := WeekdayBounds(day)

		// First: check if there are any slots open at this timeslot
		timeslotAppointments, err := sa.GetAppointmentsByTimeslot(r.Context(), q.ID, start, end, timeslot)
		if err != nil {
			l.Errorw("failed to get appointments for timeslot", "err", err)
			s.internalServerError(w, r)
			return
		}

		open := int(schedule.Schedule[timeslot] - '0')
		for _, a := range timeslotAppointments {
			if a.StudentEmail != nil {
				open--
			}
		}

		if open < 1 {
			l.Warnw("no appointment slots available at timeslot")
			s.errorMessage(
				http.StatusConflict,
				"There are no slots open at that time!",
				w, r,
			)
			return
		}

		// Check if the user has an appointment starting in the future
		// (or in the previous duration minutes, meaning they have an ongoing appointment)
		startFutureCheck := time.Now().Add(-time.Duration(schedule.Duration) * time.Minute)
		appointments, err := sa.GetAppointmentsForUser(r.Context(), q.ID, startFutureCheck, BigTime(), email)
		if err != nil {
			l.Errorw("failed to get future appointments for user", "err", err)
			s.internalServerError(w, r)
			return
		}

		if len(appointments) > 0 {
			l.Warn("user attempted to sign up for appointment with one in future")
			s.errorMessage(
				http.StatusConflict,
				"You already have an appointment in the future!",
				w, r,
			)
			return
		}

		// Force some values that were previously validated by middleware
		appointment.Queue = q.ID
		appointment.Timeslot = timeslot
		appointment.ScheduledTime = start.Add(time.Duration(timeslot*schedule.Duration) * time.Minute)
		appointment.Duration = schedule.Duration
		appointment.StudentEmail = &email

		var zero float32
		if appointment.MapX == nil {
			appointment.MapX = &zero
		}
		if appointment.MapY == nil {
			appointment.MapY = &zero
		}

		newAppointment, err := sa.SignupForAppointment(r.Context(), q.ID, &appointment)
		if err != nil {
			l.Errorw("failed to sign up for appointment", "err", err)
			s.internalServerError(w, r)
			return
		}

		l.Infow("new appointment sign up", "appointment_id", newAppointment.ID)
		s.sendResponse(http.StatusCreated, newAppointment, w, r)
	}
}

type removeAppointmentSignup interface {
	RemoveAppointmentSignup(ctx context.Context, appointment ksuid.KSUID) error
}

type updateAppointment interface {
	getAppointmentsByTimeslot
	getAppointmentScheduleForDay
	signupForAppointment
	removeAppointmentSignup
	UpdateAppointment(ctx context.Context, appointment ksuid.KSUID, newAppointment *AppointmentSlot) error
}

func (s *Server) UpdateAppointment(ua updateAppointment) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := r.Context().Value(appointmentContextKey).(*AppointmentSlot)
		email := r.Context().Value(emailContextKey).(string)
		l := s.logger.With(
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"appointment_id", a.ID,
			"email", email,
		)

		if a.StudentEmail == nil {
			l.Warnw("attempted to update deleted appointment", "appointment_id", a.ID)
			s.errorMessage(
				http.StatusNotFound,
				"This appointment doesn't exist. Perhaps it was already deleted?",
				w, r,
			)
			return
		}

		if *a.StudentEmail != email {
			l.Warnw("user attempted to update appointment with other email",
				"expected_email", *a.StudentEmail,
			)
			s.errorMessage(
				http.StatusForbidden,
				"You can't update someone else's appointment!",
				w, r,
			)
			return
		}

		var newAppointment AppointmentSlot
		err := json.NewDecoder(r.Body).Decode(&newAppointment)
		if err != nil {
			l.Warnw("failed to decode appointment", "err", err)
			s.errorMessage(
				http.StatusBadRequest,
				"We couldn't read your appointment in the request body.",
				w, r,
			)
			return
		}

		if newAppointment.Description == nil || newAppointment.Name == nil || newAppointment.Location == nil ||
			*newAppointment.Description == "" || *newAppointment.Name == "" || *newAppointment.Location == "" {
			l.Warnw("got incomplete appointment", "appointment", newAppointment)
			s.errorMessage(
				http.StatusBadRequest,
				"It looks like you left out some fields in the appointment.",
				w, r,
			)
			return
		}

		newAppointment.Queue = a.Queue
		newAppointment.Duration = a.Duration
		newAppointment.StudentEmail = &email

		var zero float32
		if newAppointment.MapX == nil {
			newAppointment.MapX = &zero
		}
		if newAppointment.MapY == nil {
			newAppointment.MapY = &zero
		}

		// We're not changing any times; simple.
		if newAppointment.Timeslot == a.Timeslot {
			err = ua.UpdateAppointment(r.Context(), a.ID, &newAppointment)
			if err != nil {
				l.Errorw("failed to update appointment", "err", err)
				s.internalServerError(w, r)
				return
			}
			l.Infow("updated appointment")

			s.sendResponse(http.StatusNoContent, nil, w, r)
			return
		}

		// We're changing the appointment time. Not so simple.
		day := int(time.Now().Local().Weekday())
		start, end := WeekdayBounds(day)
		newTime := start.Add(time.Duration(a.Duration*newAppointment.Timeslot) * time.Minute)
		newAppointment.ScheduledTime = newTime

		// If the new time is in the past, stop.
		if time.Now().After(newTime) {
			l.Warnw("user attempted to change appointment to past", "new_time", newTime)
			s.errorMessage(
				http.StatusBadRequest,
				"You can't change your appointment to the past! Let us know if you have a time machine.",
				w, r,
			)
			return
		}

		schedule, err := ua.GetAppointmentScheduleForDay(r.Context(), a.Queue, day)
		if err != nil {
			l.Errorw("failed to get appointment schedule", "err", err)
			s.internalServerError(w, r)
			return
		}

		if newAppointment.Timeslot > len(schedule.Schedule) {
			l.Warnw("attempted to change appointment to non-existent timeslot",
				"timeslot", newAppointment.Timeslot,
				"num_slots", len(schedule.Schedule),
			)
			s.errorMessage(
				http.StatusNotFound,
				"That timeslot doesn't exist!",
				w, r,
			)
			return
		}

		timeslotAppointments, err := ua.GetAppointmentsByTimeslot(r.Context(), a.Queue, start, end, newAppointment.Timeslot)
		if err != nil {
			l.Errorw("failed to get appointments for timeslot", "timeslot", newAppointment.Timeslot, "err", err)
			s.internalServerError(w, r)
			return
		}

		open := int(schedule.Schedule[newAppointment.Timeslot] - '0')
		for _, a := range timeslotAppointments {
			if a.StudentEmail != nil {
				open--
			}
		}

		if open < 1 {
			l.Warnw("no appointment slots available at timeslot", "timeslot", newAppointment.Timeslot)
			s.errorMessage(
				http.StatusConflict,
				"There are no slots open at that time!",
				w, r,
			)
			return
		}

		// Add first so student doesn't lose appointment if the add fails
		createdAppointment, err := ua.SignupForAppointment(r.Context(), a.Queue, &newAppointment)
		if err != nil {
			l.Errorw("failed to create new appointment for update", "err", err)
			s.internalServerError(w, r)
			return
		}
		l.Infow("created appointment for update", "new_appointment_id", createdAppointment.ID)

		// If adding the new appointment succeeded, ditch the old one.
		err = ua.RemoveAppointmentSignup(r.Context(), a.ID)
		if err != nil {
			l.Errorw("failed to remove appointment for update", "err", err)
			s.internalServerError(w, r)
			return
		}
		l.Infow("removed appointment for update")

		s.sendResponse(http.StatusCreated, createdAppointment, w, r)
	}
}

func (s *Server) RemoveAppointmentSignup(rs removeAppointmentSignup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := r.Context().Value(appointmentContextKey).(*AppointmentSlot)
		email := r.Context().Value(emailContextKey).(string)
		l := s.logger.With(
			RequestIDContextKey, r.Context().Value(RequestIDContextKey),
			"appointment_id", a.ID,
			"email", email,
		)

		if a.StudentEmail == nil {
			l.Warnw("attempted to remove signup for already deleted appointment")
			// Return 200 for idempotency---if someone tries to delete an appointment
			// twice, the second request still had the intended effect
			w.WriteHeader(http.StatusOK)
			return
		}

		if *a.StudentEmail != email {
			l.Warnw("user attempted to delete appointment with other email",
				"expected_email", *a.StudentEmail,
			)
			s.errorMessage(
				http.StatusForbidden,
				"You can't delete someone else's appointment!",
				w, r,
			)
			return
		}

		// If an appointment happened, it happened. How did people do this in Spring D:
		if time.Now().After(a.ScheduledTime) {
			l.Warnw("user attempted to delete appointment in the past")
			s.errorMessage(
				http.StatusBadRequest,
				"You can't delete an appointment that already happened! Let's try not to cause a paradox here.",
				w, r,
			)
			return
		}

		err := rs.RemoveAppointmentSignup(r.Context(), a.ID)
		if err != nil {
			l.Errorw("failed to remove signup for appointment", "err", err)
			s.internalServerError(w, r)
			return
		}

		l.Infow("removed signup for appointment")
		s.sendResponse(http.StatusNoContent, nil, w, r)
	}
}