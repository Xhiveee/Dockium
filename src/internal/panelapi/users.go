package panelapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/auth"
	"dockium/internal/db"
	"dockium/internal/present"
)

func (s *Server) ListUsers(ctx context.Context, request *connect.Request[panelv1.Page]) (*connect.Response[panelv1.UserList], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit, offset := normalizePage(request.Msg)
	users, err := s.store.ListUsers(ctx, limit, offset)
	if err != nil {
		return nil, rpcError(err, "не удалось получить пользователей")
	}
	total, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить пользователей")
	}
	items := make([]*panelv1.User, 0, len(users))
	for _, user := range users {
		items = append(items, present.User(user, false))
	}
	return connect.NewResponse(&panelv1.UserList{Items: items, Total: uint32(total)}), nil
}

func (s *Server) CreateUser(ctx context.Context, request *connect.Request[panelv1.CreateUserRequest]) (*connect.Response[panelv1.User], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	login := strings.TrimSpace(request.Msg.Login)
	role := request.Msg.Role
	if role == "" {
		role = db.RoleUser
	}
	if role != db.RoleAdmin && role != db.RoleUser {
		return nil, invalid("неизвестная роль")
	}
	if err := auth.ValidateLogin(login); err != nil {
		return nil, invalid(err.Error())
	}
	if err := auth.ValidatePassword(request.Msg.Password, login); err != nil {
		return nil, invalid(err.Error())
	}
	hash, err := auth.HashPassword(request.Msg.Password)
	if err != nil {
		return nil, rpcError(err, "не удалось создать пользователя")
	}
	user, err := s.store.CreateUser(ctx, login, hash, role)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("логин уже занят"))
		}
		return nil, rpcError(err, "не удалось создать пользователя")
	}
	return connect.NewResponse(present.User(user, false)), nil
}

func (s *Server) UpdateUser(ctx context.Context, request *connect.Request[panelv1.UpdateUserRequest]) (*connect.Response[panelv1.User], error) {
	state, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.store.UserByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "пользователь не найден")
	}
	role := request.Msg.Role
	if role == "" {
		role = user.Role
	}
	if role != db.RoleAdmin && role != db.RoleUser {
		return nil, invalid("неизвестная роль")
	}
	if user.Role == db.RoleAdmin && role != db.RoleAdmin {
		admins, err := s.store.CountAdmins(ctx)
		if err != nil {
			return nil, rpcError(err, "не удалось проверить администраторов")
		}
		if admins <= 1 {
			return nil, failedPrecondition("нельзя снять роль с последнего администратора")
		}
	}
	if user.ID == state.user.ID && role != db.RoleAdmin {
		return nil, failedPrecondition("нельзя снять роль с самого себя")
	}
	var hash *string
	if request.Msg.Password != "" {
		if err := auth.ValidatePassword(request.Msg.Password, user.Login); err != nil {
			return nil, invalid(err.Error())
		}
		value, err := auth.HashPassword(request.Msg.Password)
		if err != nil {
			return nil, rpcError(err, "не удалось изменить пароль")
		}
		hash = &value
	}
	if err := s.store.UpdateUser(ctx, user.ID, role, hash); err != nil {
		return nil, rpcError(err, "не удалось изменить пользователя")
	}
	if hash != nil {
		if err := s.store.DeleteUserSessions(ctx, user.ID, ""); err != nil {
			s.log.Warn("delete sessions after password change", "error", err)
		}
	}
	user.Role = role
	return connect.NewResponse(present.User(user, false)), nil
}

func (s *Server) DeleteUser(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	state, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.store.UserByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "пользователь не найден")
	}
	if user.ID == state.user.ID {
		return nil, failedPrecondition("нельзя удалить собственную учётную запись")
	}
	if user.Role == db.RoleAdmin {
		admins, err := s.store.CountAdmins(ctx)
		if err != nil {
			return nil, rpcError(err, "не удалось проверить администраторов")
		}
		if admins <= 1 {
			return nil, failedPrecondition("нельзя удалить последнего администратора")
		}
	}
	if err := s.store.DeleteUser(ctx, user.ID); err != nil {
		return nil, rpcError(err, "не удалось удалить пользователя")
	}
	return emptyResponse(), nil
}

func (s *Server) ListAccess(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[panelv1.UserAccess], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if _, err := s.store.UserByID(ctx, request.Msg.Id); err != nil {
		return nil, rpcError(err, "пользователь не найден")
	}
	items, err := s.store.ListAccess(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "не удалось получить доступы")
	}
	return connect.NewResponse(&panelv1.UserAccess{Items: accessProto(items)}), nil
}

func (s *Server) SetAccess(ctx context.Context, request *connect.Request[panelv1.SetAccessRequest]) (*connect.Response[panelv1.UserAccess], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	user, err := s.store.UserByID(ctx, request.Msg.UserId)
	if err != nil {
		return nil, rpcError(err, "пользователь не найден")
	}
	if user.Role == db.RoleAdmin {
		return nil, failedPrecondition("администратору доступны все экземпляры")
	}
	access := make([]db.InstanceAccess, 0, len(request.Msg.Items))
	for _, item := range request.Msg.Items {
		if item.InstanceId <= 0 {
			continue
		}
		if _, err := s.store.InstanceByID(ctx, item.InstanceId); err != nil {
			return nil, invalid("экземпляр не найден")
		}
		access = append(access, db.InstanceAccess{
			InstanceID: item.InstanceId,
			Control:    item.Control,
			Console:    item.Console,
			Files:      item.Files,
		})
	}
	if err := s.store.SetAccess(ctx, user.ID, access); err != nil {
		return nil, rpcError(err, "не удалось сохранить доступы")
	}
	return connect.NewResponse(&panelv1.UserAccess{Items: accessProto(access)}), nil
}

func accessProto(items []db.InstanceAccess) []*panelv1.InstanceAccess {
	result := make([]*panelv1.InstanceAccess, 0, len(items))
	for _, item := range items {
		result = append(result, &panelv1.InstanceAccess{
			InstanceId: item.InstanceID,
			Control:    item.Control,
			Console:    item.Console,
			Files:      item.Files,
		})
	}
	return result
}
